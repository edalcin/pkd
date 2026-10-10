package integration_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// tiny1x1PNGBase64 is a valid, minimal 1x1 transparent PNG.
const tiny1x1PNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func documentBearerRequest(t *testing.T, body map[string]interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/documents", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-import-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// TestCreateDocumentWithImageAttachment verifies ADR-003: a Documento created
// via Bearer with an image attachment has a body that embeds an <img> tag
// pointing at a real, newly created PKD attachment (not a data: URI).
func TestCreateDocumentWithImageAttachment(t *testing.T) {
	resp := documentBearerRequest(t, map[string]interface{}{
		"title":   "Nota com Imagem",
		"content": "<p>Texto da nota</p>",
		"tags":    []string{"trabalho"},
		"attachments": []map[string]interface{}{
			{"filename": "foto.png", "mime_type": "image/png", "data_base64": tiny1x1PNGBase64},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", resp.StatusCode)
	}

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	bodyHTML, _ := doc["body_html"].(string)
	if !strings.Contains(bodyHTML, "<img src=\"/api/attachments/") {
		t.Errorf("body_html should embed an <img> pointing at a PKD attachment URL, got: %s", bodyHTML)
	}
	if strings.Contains(bodyHTML, "data:image") {
		t.Errorf("body_html should not contain a data: URI (sanitizer strips it silently), got: %s", bodyHTML)
	}
	if !strings.Contains(bodyHTML, "Texto da nota") {
		t.Errorf("body_html should still contain the original text, got: %s", bodyHTML)
	}

	// The attachment must be independently fetchable (real stored file, not inline data).
	docID := doc["id"].(float64)
	client := loginClient(t)
	listResp := apiGet(t, client, "/api/documents/"+itoa(int64(docID))+"/attachments")
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list attachments: want 200, got %d", listResp.StatusCode)
	}
	var atts []map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&atts)
	if len(atts) != 1 {
		t.Fatalf("expected 1 stored attachment, got %d", len(atts))
	}
}

// TestCreateDocumentWithOversizedAttachment_RollsBackDocument verifies ADR-003 D3:
// when an attachment fails to import, the whole document is rolled back —
// no partially-created document is left behind.
func TestCreateDocumentWithOversizedAttachment_RollsBackDocument(t *testing.T) {
	// MaxImageMB is 10 in the test config; base64-encode ~11MB of raw bytes.
	oversized := base64.StdEncoding.EncodeToString(make([]byte, 11*1024*1024))

	resp := documentBearerRequest(t, map[string]interface{}{
		"title":   "Nota Que Deve Ser Revertida",
		"content": "<p>não deve sobreviver</p>",
		"attachments": []map[string]interface{}{
			{"filename": "grande.png", "mime_type": "image/png", "data_base64": oversized},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("create: want 413, got %d", resp.StatusCode)
	}

	client := loginClient(t)
	treeResp := apiGet(t, client, "/api/tree")
	defer treeResp.Body.Close()
	body := new(bytes.Buffer)
	body.ReadFrom(treeResp.Body)
	if strings.Contains(body.String(), "Nota Que Deve Ser Revertida") {
		t.Error("rolled-back document should not appear in the tree — rollback did not delete it")
	}
}

// TestCreateDocumentBearer_IdempotentAndSearchable: the same idempotency_key
// never creates a duplicate, no tag is forced, and the new Documento is in
// the lexical search at once (the old /api/import needed a restart).
func TestCreateDocumentBearer_IdempotentAndSearchable(t *testing.T) {
	body := map[string]interface{}{
		"title":           "Receita do Hermes",
		"content":         "<p>palavraunicaxyz</p>",
		"tags":            []string{"cozinha"},
		"idempotency_key": "doc-key-1",
	}
	first := documentBearerRequest(t, body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first create: want 201, got %d", first.StatusCode)
	}
	var doc map[string]interface{}
	json.NewDecoder(first.Body).Decode(&doc)

	second := documentBearerRequest(t, body)
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("repeat create: want 200, got %d", second.StatusCode)
	}
	var again map[string]interface{}
	json.NewDecoder(second.Body).Decode(&again)
	if again["id"] != doc["id"] {
		t.Errorf("repeat create returned id %v, want %v", again["id"], doc["id"])
	}

	tags, _ := doc["tags"].([]interface{})
	if len(tags) != 1 || tags[0] != "cozinha" {
		t.Errorf("tags = %v, want only [cozinha]", doc["tags"])
	}

	client := loginClient(t)
	resp := apiGet(t, client, "/api/search?q=palavraunicaxyz")
	defer resp.Body.Close()
	found := new(bytes.Buffer)
	found.ReadFrom(resp.Body)
	if !strings.Contains(found.String(), "Receita do Hermes") {
		t.Errorf("new Documento not in search right after create: %s", found.String())
	}
}
