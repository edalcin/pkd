package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Chat models. Whitelist compilada de propósito: listar GET /v1beta/models em
// tempo real trouxe ~40 modelos irrelevantes (embedding, TTS, imagem, previews)
// e uma chamada de rede que pode falhar. Ver ADR-006 D9.
//
// Não existe modelo Pro em GA hoje: gemini-3.1-pro é preview, e a família 2.0
// foi deprecada em 01/06/2026. O preview é oferecido porque um dropdown de uma
// única opção não é uma escolha, e porque aqui a falha é VISÍVEL (o chat
// responde com erro) — diferente da busca semântica, que degradava em silêncio
// e foi o que motivou ADR-004 D7.
const (
	ChatModelFlash = "models/gemini-3.7-flash"
	ChatModelPro   = "models/gemini-3.1-pro"

	DefaultChatModel = ChatModelFlash
)

// IsValidChatModel reports whether m is in the compiled whitelist.
func IsValidChatModel(m string) bool {
	return m == ChatModelFlash || m == ChatModelPro
}

// geminiBaseURL is the single definition of the API root, shared by the
// embedding and the generation callers so the two can never drift.
// A var, not a const, only so tests can point the client at a fake server;
// production never reassigns it.
var geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/"

// SetGeminiBaseURLForTest redirects every Gemini call and returns a func that
// restores the previous value. Test-only; there is no production caller.
func SetGeminiBaseURLForTest(u string) func() {
	prev := geminiBaseURL
	geminiBaseURL = u
	return func() { geminiBaseURL = prev }
}

// geminiURL builds the endpoint for a model + method. sse=true appends
// ?alt=sse, required for streamGenerateContent to emit Server-Sent Events
// instead of a JSON array.
func geminiURL(model, method string, sse bool) string {
	u := geminiBaseURL + model + ":" + method
	if sse {
		u += "?alt=sse"
	}
	return u
}

// setGeminiAuth authenticates a Gemini request by header. The ?key= query
// param also works but leaks the credential into proxy and access logs, so it
// is not used anywhere in this package. See ADR-006 D10.
func setGeminiAuth(req *http.Request, apiKey string) {
	req.Header.Set("x-goog-api-key", apiKey)
}

// ChatDoc is one grounding document: the whole body goes into the prompt, no
// chunking. See ADR-006 D2.
type ChatDoc struct {
	ID    int64
	Title string
	Body  string
}

// ChatMessage is one conversation turn. Role is "user" or "model" — the roles
// the Gemini API accepts in contents[].
type ChatMessage struct {
	Role string
	Text string
}

// chatStreamTimeout bounds a whole generation. Generous because a long answer
// over eight documents legitimately takes a while; the caller's context still
// cancels earlier when the browser disconnects.
const chatStreamTimeout = 5 * time.Minute

// StreamChat calls streamGenerateContent and invokes onChunk for every text
// fragment as it arrives. The grounding documents are prepended to the first
// user turn rather than sent as a separate role: the API only accepts "user"
// and "model" in contents[], and systemInstruction carries the rules, not the
// data.
//
// ctx cancellation aborts the HTTP request, which stops the generation upstream
// instead of leaving it running and billing (ADR-006 D7).
func StreamChat(ctx context.Context, apiKey, model, systemPrompt string,
	docs []ChatDoc, msgs []ChatMessage, maxOutputTokens int, onChunk func(string) error) error {

	if apiKey == "" {
		return fmt.Errorf("chat: missing apiKey")
	}
	if !IsValidChatModel(model) {
		model = DefaultChatModel
	}

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role,omitempty"`
		Parts []part `json:"parts"`
	}
	type genConfig struct {
		// temperature is deliberately absent: the Gemini 3.x docs warn that
		// moving it off the default degrades reasoning (ADR-006 D12).
		MaxOutputTokens int `json:"maxOutputTokens"`
	}
	type reqBody struct {
		SystemInstruction *content  `json:"systemInstruction,omitempty"`
		Contents          []content `json:"contents"`
		GenerationConfig  genConfig `json:"generationConfig"`
	}

	contents := make([]content, 0, len(msgs))
	grounding := formatChatDocs(docs)
	for i, m := range msgs {
		text := m.Text
		if i == 0 && grounding != "" {
			text = grounding + "\n\nPergunta: " + text
		}
		contents = append(contents, content{Role: m.Role, Parts: []part{{Text: text}}})
	}

	body, err := json.Marshal(reqBody{
		SystemInstruction: &content{Parts: []part{{Text: systemPrompt}}},
		Contents:          contents,
		GenerationConfig:  genConfig{MaxOutputTokens: maxOutputTokens},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		geminiURL(model, "streamGenerateContent", true), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	setGeminiAuth(req, apiKey)

	resp, err := (&http.Client{Timeout: chatStreamTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gemini chat: status %d: %s", resp.StatusCode, snippet)
	}

	// SSE frames are "data: {json}" lines separated by blank lines. Scanner
	// with a large buffer: a single chunk can carry a long paragraph.
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A malformed frame is not worth aborting a good answer over.
			continue
		}
		for _, c := range chunk.Candidates {
			for _, p := range c.Content.Parts {
				if p.Text == "" {
					continue
				}
				if err := onChunk(p.Text); err != nil {
					return err
				}
			}
			// finishReason may be ABSENT on the last frame (empty "thinking"
			// content), so it is never used as the end-of-stream signal —
			// the closed body is. It is only inspected to report truncation.
			if c.FinishReason == "SAFETY" {
				return fmt.Errorf("gemini chat: blocked by safety filters")
			}
		}
	}
	return sc.Err()
}

// formatChatDocs renders the grounding documents. Titles are labelled so the
// model can cite them and so IDs never leak into prose.
func formatChatDocs(docs []ChatDoc) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Documentos da base de conhecimento:\n")
	for _, d := range docs {
		b.WriteString("\n--- Documento: ")
		b.WriteString(d.Title)
		b.WriteString(" ---\n")
		b.WriteString(d.Body)
		b.WriteString("\n")
	}
	return b.String()
}
