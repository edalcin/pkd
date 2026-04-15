package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestManifestWebmanifest_Returns200(t *testing.T) {
	resp, err := http.Get(ts.URL + "/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("manifest: want 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "manifest") && !strings.Contains(ct, "json") {
		t.Errorf("manifest content-type: want manifest+json or similar, got %q", ct)
	}

	// Validate minimal manifest shape
	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatalf("manifest not valid JSON: %v", err)
	}
	for _, key := range []string{"name", "short_name", "start_url", "display", "icons"} {
		if _, ok := manifest[key]; !ok {
			t.Errorf("manifest missing field: %q", key)
		}
	}
}

func TestServiceWorker_Returns200WithNoCacheHeader(t *testing.T) {
	resp, err := http.Get(ts.URL + "/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("sw.js: want 200, got %d", resp.StatusCode)
	}

	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("sw.js Cache-Control: want no-cache, got %q", cc)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("sw.js Content-Type: want javascript, got %q", ct)
	}
}
