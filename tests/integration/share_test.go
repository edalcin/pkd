package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestShareRecursive verifica que criar um share com include_children=true
// gera auto-shares para os descendentes, e a página pública lista os filhos.
func TestShareRecursive(t *testing.T) {
	client := loginClient(t)

	// Cria documento pai
	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Share Parent",
	}))
	parentID := int64(parent["id"].(float64))

	// Cria filho
	_ = decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "Share Child",
	}))

	// Gera share recursivo (default)
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", map[string]interface{}{
		"include_children": true,
	})
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)
	if shareURL == "" {
		t.Fatal("share URL should not be empty")
	}

	// Acessa a página pública
	pubResp, err := http.Get(shareURL)
	if err != nil {
		t.Fatalf("public page GET: %v", err)
	}
	defer pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("public page: want 200, got %d", pubResp.StatusCode)
	}

	// A página deve conter o título do filho (share automático foi criado)
	bodyBytes, _ := io.ReadAll(pubResp.Body)
	if !strings.Contains(string(bodyBytes), "Share Child") {
		t.Error("public page should list child document when include_children=true")
	}
}

// TestShareNonRecursive verifica que criar um share com include_children=false
// NÃO gera auto-shares para os descendentes, e a página pública não lista filhos.
func TestShareNonRecursive(t *testing.T) {
	client := loginClient(t)

	// Cria documento pai
	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "NonRec Parent",
	}))
	parentID := int64(parent["id"].(float64))

	// Cria filho
	_ = decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "NonRec Child",
	}))

	// Gera share NÃO recursivo
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", map[string]interface{}{
		"include_children": false,
	})
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create non-recursive share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)
	if shareURL == "" {
		t.Fatal("share URL should not be empty")
	}

	// Acessa a página pública
	pubResp, err := http.Get(shareURL)
	if err != nil {
		t.Fatalf("public page GET: %v", err)
	}
	defer pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("public page: want 200, got %d", pubResp.StatusCode)
	}

	// A página NÃO deve conter o título do filho
	bodyBytes, _ := io.ReadAll(pubResp.Body)
	if strings.Contains(string(bodyBytes), "NonRec Child") {
		t.Error("public page should NOT list child document when include_children=false")
	}
}

// TestShareDefaultIsRecursive verifica que omitir include_children no body
// resulta em comportamento recursivo (backward-compatible).
func TestShareDefaultIsRecursive(t *testing.T) {
	client := loginClient(t)

	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Default Parent",
	}))
	parentID := int64(parent["id"].(float64))

	_ = decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "Default Child",
	}))

	// POST sem body (simula cliente antigo)
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", nil)
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create default share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)
	if shareURL == "" {
		t.Fatal("share URL should not be empty")
	}

	pubResp, err := http.Get(shareURL)
	if err != nil {
		t.Fatalf("public page GET: %v", err)
	}
	defer pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("public page: want 200, got %d", pubResp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(pubResp.Body)
	if !strings.Contains(string(bodyBytes), "Default Child") {
		t.Error("public page should list child when no body sent (default=recursive)")
	}
}
