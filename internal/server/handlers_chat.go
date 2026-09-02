package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/edalcin/pkd/internal/store"
)

const (
	// chatMaxDocs caps how many documents go into a prompt. See ADR-006 D3.
	chatMaxDocs = 8

	// chatTokenBudget caps the prompt size. With whole document bodies in the
	// prompt (D2), cost per question is a function of document size, which
	// nobody controls — this is the ceiling between one question and a
	// surprise bill. Documents that do not fit are OMITTED, never truncated:
	// truncating is chunking through the back door, with the model citing a
	// document whose relevant passage may have been cut.
	chatTokenBudget = 100_000

	// chatCharsPerToken is a deliberately crude estimate. ponytail: len/4
	// beats adding a tokenizer dependency to guard a ceiling that only needs
	// to be roughly right.
	chatCharsPerToken = 4

	// chatHistoryTurns is how many trailing USER messages are concatenated
	// into the retrieval query. "E sobre a fase escura?" carries no context on
	// its own; the previous turn has it. See ADR-006 D6.
	chatHistoryTurns = 3

	// chatRelevanceFloor is the minimum cosine similarity the best semantic
	// candidate must reach before the model is called at all.
	//
	// It exists because hybrid search NEVER returns empty: LIKE matches any
	// substring and semanticQueryFloor is 0.30. "Qual a capital da França"
	// retrieves eight irrelevant documents, and the model then answers from
	// its own knowledge under a source list that supports nothing — an answer
	// that LOOKS grounded. See ADR-006 D4.
	//
	// Deliberately separate from semanticQueryFloor: reusing 0.30 would filter
	// nothing, and raising the search floor would degrade search to fix chat.
	// ponytail: 0.50 is a guess; the right value only comes from real use, and
	// it stays a constant, not a UI setting.
	chatRelevanceFloor = 0.50

	chatMaxOutputTokens = 4096
)

// chatSystemPrompt enforces strict grounding. The model must refuse rather
// than answer from its own knowledge: the boundary between "this is in my PKD"
// and "the model thinks so" is the boundary a PKM derives its value from.
const chatSystemPrompt = `Você é o assistente de uma base de conhecimento pessoal (PKD).

Responda EXCLUSIVAMENTE com base nos documentos fornecidos abaixo. Regras:
- Se os documentos não contiverem a informação pedida, diga claramente que não encontrou aquilo nos documentos. Nunca complete com conhecimento próprio.
- Ao afirmar algo, mencione o título do documento que sustenta a afirmação.
- Não invente títulos de documentos: use apenas os fornecidos.
- Responda no mesmo idioma da pergunta.`

// chatSource is one consulted document, as reported by the server. Ver
// "Documentos Consultados" no glossário: por vir do servidor e não do modelo,
// não pode ser alucinado.
type chatSource struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type chatMessage struct {
	Role string `json:"role"` // "user" | "model"
	Text string `json:"text"`
}

// handleChat answers a question grounded on the user's documents, streaming
// the answer as Server-Sent Events.
//
// POST (not GET + EventSource) because the CSRF middleware is global and
// EventSource cannot send custom headers — exempting a route from CSRF to
// accommodate a transport choice is not a trade we make. See ADR-006 D7.
func (s *Server) handleChat() http.HandlerFunc {
	type request struct {
		Messages []chatMessage `json:"messages"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		if s.cfg.GeminiAPIKey == "" {
			http.Error(w, "GEMINI_API_KEY not configured", http.StatusServiceUnavailable)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		question := strings.TrimSpace(req.Messages[len(req.Messages)-1].Text)
		if question == "" {
			http.Error(w, "empty question", http.StatusBadRequest)
			return
		}

		docs, sources, semErr, err := s.retrieveChatContext(r, req.Messages)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		// The semantic leg failing is NOT the same as nothing being relevant.
		// Search may hide the difference (RRF degrades to lexical, ADR-002 D1),
		// but the chat may not: with a broken key, "nada relevante" is a lie
		// about the corpus when the truth is that the retrieval never ran.
		// Reporting it is the whole point of D4.
		if semErr != nil {
			writeSSE(w, flusher, "sources", []chatSource{})
			writeSSE(w, flusher, "error", chatErrorMessage(semErr))
			writeSSE(w, flusher, "done", "")
			return
		}

		// No relevant document: answer without spending a model call (D4).
		if len(docs) == 0 {
			writeSSE(w, flusher, "sources", []chatSource{})
			writeSSE(w, flusher, "text", "Não encontrei nada relevante sobre isso nos seus documentos.")
			writeSSE(w, flusher, "done", "")
			return
		}
		writeSSE(w, flusher, "sources", sources)

		model, err := s.settings.ChatModel()
		if err != nil {
			model = store.DefaultChatModel
		}
		onChunk := func(text string) error { return writeSSE(w, flusher, "text", text) }
		// r.Context() is propagated: closing the tab kills the generation
		// instead of leaving it running and billing.
		if err := store.StreamChat(r.Context(), s.cfg.GeminiAPIKey, model,
			chatSystemPrompt, docs, toStoreMessages(req.Messages), chatMaxOutputTokens, onChunk); err != nil {
			log.Printf("chat: stream failed: %v", err)
			// Headers are already sent; the error can only travel as an event.
			// 429 is a message, never an automatic retry — a retry pays for
			// the same generation twice (D11).
			writeSSE(w, flusher, "error", chatErrorMessage(err))
		}
		writeSSE(w, flusher, "done", "")
	}
}

// retrieveChatContext runs the same hybrid pipeline as search (D1), applies the
// relevance floor (D4) and the token budget (D3), and returns the documents to
// ground on plus the consulted-document list for the client.
//
// The semantic error is returned SEPARATELY from the fatal error: search folds
// it into a lexical-only ranking (ADR-002 D1), but the chat must tell the user
// that retrieval never ran instead of claiming the corpus has nothing.
func (s *Server) retrieveChatContext(r *http.Request, msgs []chatMessage) (docs []store.ChatDoc, sources []chatSource, semErr error, err error) {
	q := retrievalQuery(msgs)

	lex, err := s.search.LexicalDocIDs(q)
	if err != nil {
		return nil, nil, nil, err
	}
	hits, semErr := s.links.SemanticSearchDocIDs(r.Context(), s.cfg.GeminiAPIKey, q)
	if semErr != nil {
		log.Printf("chat: semantic leg failed: %v", semErr)
		return nil, nil, semErr, nil
	}
	// The floor gates on the BEST semantic candidate. hits are sorted by score
	// descending by SemanticSearchDocIDs. This is the first real consumer of
	// SemanticHit.Score, which ADR-004 left as a write-only extension point.
	if len(hits) == 0 || hits[0].Score < chatRelevanceFloor {
		return nil, nil, nil, nil
	}
	semIDs := make([]int64, len(hits))
	for i, h := range hits {
		semIDs[i] = h.DocID
	}

	fused := store.FuseRRF(lex, semIDs, hybridResultLimit)
	if len(fused) == 0 {
		return nil, nil, nil, nil
	}
	// view "all": chat spans active + archived, exactly like search (D8 of
	// ADR-002 established that universe).
	found, err := s.docs.ListByIDsFiltered(fused, "all", nil, false)
	if err != nil {
		return nil, nil, nil, err
	}
	byID := make(map[int64]string, len(found))
	titles := make(map[int64]string, len(found))
	for _, d := range found {
		// Encrypted bodies are ciphertext — they embed to noise and would be
		// noise in a prompt too. EmbedStaleDocs already excludes them; the
		// lexical leg does not, so exclude here.
		if d.Encrypted {
			continue
		}
		byID[d.ID] = d.BodyText
		titles[d.ID] = d.Title
	}

	docs = make([]store.ChatDoc, 0, chatMaxDocs)
	sources = make([]chatSource, 0, chatMaxDocs)
	budget := chatTokenBudget
	for _, id := range fused {
		if len(docs) >= chatMaxDocs {
			break
		}
		body, ok := byID[id]
		if !ok {
			continue
		}
		cost := (len(body) + len(titles[id])) / chatCharsPerToken
		if cost > budget {
			continue // omitted, never truncated (D3)
		}
		budget -= cost
		docs = append(docs, store.ChatDoc{ID: id, Title: titles[id], Body: body})
		sources = append(sources, chatSource{ID: id, Title: titles[id]})
	}
	return docs, sources, nil, nil
}

// retrievalQuery concatenates the trailing user messages so a follow-up like
// "e sobre a fase escura?" retrieves with the subject from earlier turns (D6).
// A concatenated query is poor for embedding and excellent for FTS5, and RRF
// already fuses two lists of unequal quality — the hybrid architecture of
// ADR-002 pays a dividend here.
func retrievalQuery(msgs []chatMessage) string {
	parts := make([]string, 0, chatHistoryTurns)
	for i := len(msgs) - 1; i >= 0 && len(parts) < chatHistoryTurns; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		if t := strings.TrimSpace(msgs[i].Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func toStoreMessages(msgs []chatMessage) []store.ChatMessage {
	out := make([]store.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role != "model" {
			role = "user"
		}
		if t := strings.TrimSpace(m.Text); t != "" {
			out = append(out, store.ChatMessage{Role: role, Text: t})
		}
	}
	return out
}

// chatErrorMessage turns an upstream failure into something a user can act on.
// API_KEY_INVALID is checked BEFORE the generic 400: Gemini reports a bad key
// as 400 INVALID_ARGUMENT, and "a pergunta excedeu o limite" would send the
// user chasing the wrong problem — observed in a smoke test against a stale
// key.
func chatErrorMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "API_KEY_INVALID"), strings.Contains(msg, "API key not valid"):
		return "A GEMINI_API_KEY do servidor é inválida ou expirou. Verifique a variável de ambiente."
	case strings.Contains(msg, "status 429"):
		return "Limite de uso da API do Gemini atingido. Tente novamente em alguns minutos."
	case strings.Contains(msg, "status 400"):
		return "A pergunta com os documentos recuperados excedeu o limite do modelo. Tente uma pergunta mais específica."
	case strings.Contains(msg, "status 404"):
		return "O modelo de chat configurado não está disponível. Escolha outro em Administração → Preferências."
	default:
		return "Falha ao gerar a resposta."
	}
}

// writeSSE emits one Server-Sent Event and flushes it immediately.
func writeSSE(w http.ResponseWriter, f http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(data) + "\n\n")); err != nil {
		return err
	}
	f.Flush()
	return nil
}
