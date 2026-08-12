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

func importRequest(t *testing.T, body map[string]interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/import", bytes.NewReader(b))
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

// TestImportWithImageAttachment verifies ADR-003: an imported note with an
// image attachment creates a document whose body embeds an <img> tag
// pointing at a real, newly created PKD attachment (not a data: URI).
func TestImportWithImageAttachment(t *testing.T) {
	resp := importRequest(t, map[string]interface{}{
		"title":   "Nota com Imagem",
		"content": "<p>Texto da nota</p>",
		"tags":    []string{"trabalho"},
		"attachments": []map[string]interface{}{
			{"filename": "foto.png", "mime_type": "image/png", "data_base64": tiny1x1PNGBase64},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import: want 201, got %d", resp.StatusCode)
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
		t.Errorf("body_html should still contain the original note text, got: %s", bodyHTML)
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

// TestImportWithOversizedAttachment_RollsBackDocument verifies ADR-003 D3:
// when an attachment fails to import, the whole document is rolled back —
// no partially-imported document is left behind.
func TestImportWithOversizedAttachment_RollsBackDocument(t *testing.T) {
	// MaxImageMB is 10 in the test config; base64-encode ~11MB of raw bytes.
	oversized := base64.StdEncoding.EncodeToString(make([]byte, 11*1024*1024))

	resp := importRequest(t, map[string]interface{}{
		"title":   "Nota Que Deve Ser Revertida",
		"content": "<p>não deve sobreviver</p>",
		"attachments": []map[string]interface{}{
			{"filename": "grande.png", "mime_type": "image/png", "data_base64": oversized},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("import: want 413, got %d", resp.StatusCode)
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
