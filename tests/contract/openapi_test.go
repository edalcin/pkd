package contract_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/server"
	"github.com/edalcin/pkd/internal/sessions"
	"github.com/edalcin/pkd/internal/store"
)

// testServer is a shared test server backed by an in-memory SQLite database.
// Subtests access it via the package-level ts variable.
var ts *httptest.Server

// doc is the loaded OpenAPI 3.0 document used by validateResponse.
var doc *openapi3.T

func TestMain(m *testing.M) {
	// Open an in-memory SQLite database (shared cache so concurrent reads work).
	db, err := store.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		panic("contract tests: could not open in-memory DB: " + err.Error())
	}

	cfg := &config.Config{
		Password:           "testpassword",
		DBPath:             ":memory:",
		AttachmentsPath:    os.TempDir(),
		ListenAddr:         ":0",
		SessionIdleMinutes: 60,
		MaxImageMB:         10,
		MaxAttachmentMB:    100,
	}

	sess := sessions.New(60)
	srv := server.New(cfg, db, sess)
	ts = httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Load the OpenAPI spec so subtests can validate against it.
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err = loader.LoadFromFile("openapi.yaml")
	if err != nil {
		panic("contract tests: could not load openapi.yaml: " + err.Error())
	}
	if err := doc.Validate(loader.Context); err != nil {
		panic("contract tests: openapi.yaml is invalid: " + err.Error())
	}

	os.Exit(m.Run())
}

// validateResponse checks that the response status matches the expected code.
// A future enhancement would validate the response body against the schema.
func validateResponse(t *testing.T, resp *http.Response, expectedStatus int) {
	t.Helper()
	if resp.StatusCode != expectedStatus {
		t.Errorf("expected status %d, got %d", expectedStatus, resp.StatusCode)
	}
}

// TestHealthzEndpoint verifies the /healthz endpoint returns 200.
func TestHealthzEndpoint(t *testing.T) {
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	validateResponse(t, resp, http.StatusOK)
}

// TestSecurityHeaders verifies the required hardening headers are present.
func TestSecurityHeaders(t *testing.T) {
	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	required := map[string]string{
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
		"Content-Security-Policy":   "", // value checked separately
	}
	for header, want := range required {
		got := resp.Header.Get(header)
		if got == "" {
			t.Errorf("missing security header: %s", header)
			continue
		}
		if want != "" && got != want {
			t.Errorf("header %s: want %q, got %q", header, want, got)
		}
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header missing")
	}
	if csp == "" || len(csp) < 20 {
		t.Errorf("Content-Security-Policy looks too short: %q", csp)
	}
}

// TestCSRFCookieSet verifies a CSRF cookie is set on GET /login.
func TestCSRFCookieSet(t *testing.T) {
	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "pkd_csrf" {
			found = true
			if len(c.Value) < 10 {
				t.Errorf("pkd_csrf cookie value too short: %q", c.Value)
			}
		}
	}
	if !found {
		t.Error("pkd_csrf cookie not set on GET /login")
	}
}

// TestOpenAPIDocLoaded is a smoke test confirming the spec loaded correctly.
func TestOpenAPIDocLoaded(t *testing.T) {
	if doc == nil {
		t.Fatal("OpenAPI document not loaded")
	}
	if doc.Info == nil || doc.Info.Title == "" {
		t.Error("OpenAPI document has no title")
	}
	t.Logf("loaded OpenAPI spec: %s v%s", doc.Info.Title, doc.Info.Version)
}
