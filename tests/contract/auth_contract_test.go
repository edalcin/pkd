package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// newClient returns an http.Client with a cookie jar (needed for CSRF double-submit).
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// fetchCSRF does a GET /login with the given client to seed the pkd_csrf cookie,
// then extracts and returns the token value.
func fetchCSRF(t *testing.T, client *http.Client) string {
	t.Helper()
	resp, err := client.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "pkd_csrf" {
			return c.Value
		}
	}
	t.Fatal("no pkd_csrf cookie after GET /login")
	return ""
}

func TestLoginSuccess_ReturnsNoContent(t *testing.T) {
	client := newClient(t)
	csrf := fetchCSRF(t, client)

	body, _ := json.Marshal(map[string]string{"password": "testpassword"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	validateResponse(t, resp, http.StatusNoContent)
}

func TestLoginWrongPassword_Returns401(t *testing.T) {
	client := newClient(t)
	csrf := fetchCSRF(t, client)

	body, _ := json.Marshal(map[string]string{"password": "wrongpassword"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	validateResponse(t, resp, http.StatusUnauthorized)
}

func TestCSRFMissing_Returns403(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"password": "testpassword"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-CSRF-Token header — must be rejected

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	validateResponse(t, resp, http.StatusForbidden)
}

func TestUnauthenticatedTree_Returns401(t *testing.T) {
	resp, err := http.Get(ts.URL + "/api/tree")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	validateResponse(t, resp, http.StatusUnauthorized)
}
