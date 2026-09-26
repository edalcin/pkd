package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func noteBearerRequest(t *testing.T, method, path string, body interface{}) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(b))
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

// Q11: explicit created_at is honored via Bearer, but ignored via session
// (the server clock wins instead).
func TestCreateNote_CreatedAtBearerVsSession(t *testing.T) {
	resp := noteBearerRequest(t, http.MethodPost, "/api/notes", map[string]interface{}{
		"title":      "Nota via Hermes",
		"content":    "<p>corpo</p>",
		"tags":       []string{"x"},
		"created_at": "2019-03-04T05:06:07Z",
		"updated_at": "2020-01-02T03:04:05Z",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bearer create: want 201, got %d", resp.StatusCode)
	}
	var bearerDoc map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&bearerDoc)
	if got := bearerDoc["created_at"]; got != "2019-03-04T05:06:07Z" {
		t.Errorf("bearer created_at = %v, want honored 2019-03-04T05:06:07Z", got)
	}
	// Writing the body/tags after the insert must not bump the explicit updated_at.
	if got := bearerDoc["updated_at"]; got != "2020-01-02T03:04:05Z" {
		t.Errorf("bearer updated_at = %v, want honored 2020-01-02T03:04:05Z", got)
	}

	client := loginClient(t)
	sessionResp := apiPost(t, client, "/api/notes", map[string]interface{}{
		"title":      "Nota via UI",
		"created_at": "2019-03-04T05:06:07Z",
	})
	defer sessionResp.Body.Close()
	if sessionResp.StatusCode != http.StatusCreated {
		t.Fatalf("session create: want 201, got %d", sessionResp.StatusCode)
	}
	var sessionDoc map[string]interface{}
	json.NewDecoder(sessionResp.Body).Decode(&sessionDoc)
	if got, _ := sessionDoc["created_at"].(string); got == "2019-03-04T05:06:07Z" {
		t.Errorf("session created_at must NOT honor the client value, got %v", got)
	}
}

// Q3/Q17: converting a Nota to a Memória validates the date before writing;
// an impossible date is rejected with 400.
func TestConvertNote_ToMemory_InvalidDate(t *testing.T) {
	client := loginClient(t)
	createResp := apiPost(t, client, "/api/notes", map[string]interface{}{"title": "Nota para converter"})
	defer createResp.Body.Close()
	var doc map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&doc)
	id := int64(doc["id"].(float64))

	resp := apiPost(t, client, "/api/notes/"+itoa(id)+"/convert", map[string]interface{}{
		"to":   "memory",
		"date": map[string]interface{}{"year": 2026, "month": 2, "day": 31},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid date: want 400, got %d", resp.StatusCode)
	}
}
