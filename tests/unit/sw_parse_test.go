package unit_test

import (
	"os"
	"strings"
	"testing"
)

// TestServiceWorker_ReferencesAppShellAssets verifies that sw.js references
// every critical app-shell asset path. A typo here would break offline caching.
func TestServiceWorker_ReferencesAppShellAssets(t *testing.T) {
	// sw.js is embedded at internal/server/web/sw.js
	content, err := os.ReadFile("../../internal/server/web/sw.js")
	if err != nil {
		t.Fatalf("cannot read sw.js: %v", err)
	}
	sw := string(content)

	required := []string{
		"/css/app.css",
		"/js/app.js",
		"/js/tree.js",
		"/js/editor.js",
		"/js/search.js",
		"/vendor/ckeditor5/ckeditor.js",
		"/manifest.webmanifest",
		"x-pkd-offline",
		"read-only",
		"self.addEventListener", // confirms this is a service worker file
	}

	for _, path := range required {
		if !strings.Contains(sw, path) {
			t.Errorf("sw.js missing expected reference: %q", path)
		}
	}
}
