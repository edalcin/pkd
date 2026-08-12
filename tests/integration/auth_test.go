package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/server"
	"github.com/edalcin/pkd/internal/sessions"
	"github.com/edalcin/pkd/internal/store"
)

const testPassword = "integration-test-password"

var (
	ts  *httptest.Server
	srv *server.Server
)

func TestMain(m *testing.M) {
	db, err := store.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		panic(err)
	}
	cfg := &config.Config{
		Password:           testPassword,
		DBPath:             ":memory:",
		AttachmentsPath:    os.TempDir(),
		SessionIdleMinutes: 60,
		MaxImageMB:         10,
		MaxAttachmentMB:    100,
		ImportToken:        "test-import-token",
	}
	sess := sessions.New(60)
	srv = server.New(cfg, db, sess)
	ts = httptest.NewServer(srv.Handler())
	defer ts.Close()
	os.Exit(m.Run())
}

// loginClient logs in and returns an http.Client that carries the session cookie.
func loginClient(t *testing.T) *http.Client {
	t.Helper()
	jar := newJar()
	client := &http.Client{Jar: jar}

	// First GET /login to obtain CSRF cookie
	resp, err := client.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	csrfToken := csrfFromJar(jar, ts.URL)

	body, _ := json.Marshal(map[string]string{"password": testPassword})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: want 204, got %d", resp.StatusCode)
	}
	return client
}

func TestLogin_Success(t *testing.T) {
	loginClient(t) // will Fatal on wrong status
}

func TestLogin_WrongPassword(t *testing.T) {
	jar := newJar()
	client := &http.Client{Jar: jar}

	resp, _ := client.Get(ts.URL + "/login")
	resp.Body.Close()
	csrf := csrfFromJar(jar, ts.URL)

	body, _ := json.Marshal(map[string]string{"password": "wrongpass"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)

	resp, _ = client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestLogin_Throttled_After5Failures(t *testing.T) {
	// Reset throttle after test so subsequent tests are not locked out
	t.Cleanup(func() { srv.ExportThrottle().ExportReset() })

	jar := newJar()
	client := &http.Client{Jar: jar}

	resp, _ := client.Get(ts.URL + "/login")
	resp.Body.Close()
	csrf := csrfFromJar(jar, ts.URL)

	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		resp, _ = client.Do(req)
		resp.Body.Close()
	}

	// 6th attempt — even with correct password, must be throttled
	body, _ := json.Marshal(map[string]string{"password": testPassword})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, _ = client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("want 429 after 5 failures, got %d", resp.StatusCode)
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	resp, err := http.Get(ts.URL + "/api/tree")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	client := loginClient(t)
	jar := client.Jar.(*cookieJar)

	csrf := csrfFromJar(jar, ts.URL)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/logout", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, _ := client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("logout: want 204, got %d", resp.StatusCode)
	}

	// After logout, /api/tree should return 401
	resp, _ = client.Get(ts.URL + "/api/tree")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout: want 401, got %d", resp.StatusCode)
	}
}
