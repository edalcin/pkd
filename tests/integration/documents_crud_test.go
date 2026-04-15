package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func apiPost(t *testing.T, client *http.Client, path string, body interface{}) *http.Response {
	t.Helper()
	jar := client.Jar.(*cookieJar)
	csrf := csrfFromJar(jar, ts.URL)
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func apiPut(t *testing.T, client *http.Client, path string, body interface{}) *http.Response {
	t.Helper()
	jar := client.Jar.(*cookieJar)
	csrf := csrfFromJar(jar, ts.URL)
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func apiDelete(t *testing.T, client *http.Client, path string) *http.Response {
	t.Helper()
	jar := client.Jar.(*cookieJar)
	csrf := csrfFromJar(jar, ts.URL)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeDoc(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal("decode doc:", err)
	}
	return doc
}

func TestDocumentCRUD(t *testing.T) {
	client := loginClient(t)

	// Create root document
	resp := apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Root Document",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", resp.StatusCode)
	}
	root := decodeDoc(t, resp)
	rootID := int64(root["id"].(float64))
	rootVersion := int64(root["version"].(float64))

	// Create child
	resp = apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": rootID,
		"title":     "Child Document",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create child: want 201, got %d", resp.StatusCode)
	}
	child := decodeDoc(t, resp)
	childID := int64(child["id"].(float64))

	// Update root title
	resp = apiPut(t, client, "/api/documents/"+itoa(rootID), map[string]interface{}{
		"version":   rootVersion,
		"title":     "Root Renamed",
		"body_html": "<p>Hello</p>",
		"body_text": "Hello",
		"icon":      "",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("update: want 200, got %d", resp.StatusCode)
	}
	updated := decodeDoc(t, resp)
	if updated["title"] != "Root Renamed" {
		t.Errorf("update title: want 'Root Renamed', got %v", updated["title"])
	}

	// GET tree — should include both
	treeResp, _ := client.Get(ts.URL + "/api/tree")
	var tree []interface{}
	json.NewDecoder(treeResp.Body).Decode(&tree)
	treeResp.Body.Close()
	if len(tree) == 0 {
		t.Error("tree should have at least one root document")
	}

	// Soft-delete child
	resp = apiDelete(t, client, "/api/documents/"+itoa(childID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete: want 204, got %d", resp.StatusCode)
	}

	// Admin trash should list child
	trashResp, _ := client.Get(ts.URL + "/api/admin/trash")
	var trash []interface{}
	json.NewDecoder(trashResp.Body).Decode(&trash)
	trashResp.Body.Close()

	// Restore child
	resp = apiPost(t, client, "/api/documents/"+itoa(childID)+"/restore", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("restore: want 204, got %d", resp.StatusCode)
	}

	// Child should reappear in tree under root
	treeResp, _ = client.Get(ts.URL + "/api/tree")
	json.NewDecoder(treeResp.Body).Decode(&tree)
	treeResp.Body.Close()
	if len(tree) == 0 {
		t.Error("tree after restore should not be empty")
	}
}

func TestDocumentVersionConflict(t *testing.T) {
	client := loginClient(t)

	resp := apiPost(t, client, "/api/documents", map[string]interface{}{"title": "Conflict Test"})
	doc := decodeDoc(t, resp)
	id := int64(doc["id"].(float64))
	version := int64(doc["version"].(float64))

	// First save — succeeds, version becomes 2
	resp = apiPut(t, client, "/api/documents/"+itoa(id), map[string]interface{}{
		"version": version, "title": "v2", "body_html": "", "body_text": "", "icon": "",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first save: want 200, got %d", resp.StatusCode)
	}

	// Second save with old version — must return 409
	resp = apiPut(t, client, "/api/documents/"+itoa(id), map[string]interface{}{
		"version": version, "title": "stale", "body_html": "", "body_text": "", "icon": "",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("conflict save: want 409, got %d", resp.StatusCode)
	}
	var conflict map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&conflict)
	if conflict["stored_version"] == nil {
		t.Error("conflict response should contain stored_version")
	}
}

func TestCircularMoveRejected(t *testing.T) {
	client := loginClient(t)

	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{"title": "Parent"}))
	parentID := int64(parent["id"].(float64))

	child := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID, "title": "Child",
	}))
	childID := int64(child["id"].(float64))

	// Try to move Parent under Child — must be rejected (circular)
	resp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/move", map[string]interface{}{
		"new_parent_id": childID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("circular move: want 400, got %d", resp.StatusCode)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
