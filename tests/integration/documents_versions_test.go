package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func apiGet(t *testing.T, client *http.Client, path string) *http.Response {
	t.Helper()
	resp, err := client.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeVersionList(t *testing.T, resp *http.Response) []map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var list []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal("decode version list:", err)
	}
	return list
}

func decodeVersion(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var v map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal("decode version:", err)
	}
	return v
}

// saveDocVersion saves a document and returns the updated doc.
func saveDocVersion(t *testing.T, client *http.Client, id int64, version int64, title, bodyHTML, icon string) map[string]interface{} {
	t.Helper()
	resp := apiPut(t, client, fmt.Sprintf("/api/documents/%d", id), map[string]interface{}{
		"version":   version,
		"title":     title,
		"body_html": bodyHTML,
		"body_text": "",
		"icon":      icon,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("save: want 200, got %d", resp.StatusCode)
	}
	return decodeDoc(t, resp)
}

func TestDocumentVersions(t *testing.T) {
	client := loginClient(t)

	// Create a fresh document
	resp := apiPost(t, client, "/api/documents", map[string]interface{}{"title": "Versioned Doc"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", resp.StatusCode)
	}
	doc := decodeDoc(t, resp)
	id := int64(doc["id"].(float64))

	// Initially: no versions
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list versions (empty): want 200, got %d", resp.StatusCode)
	}
	list := decodeVersionList(t, resp)
	if len(list) != 0 {
		t.Fatalf("want 0 versions initially, got %d", len(list))
	}

	// First real save: creates snapshot
	doc = saveDocVersion(t, client, id, int64(doc["version"].(float64)), "Title A", "<p>Content A</p>", "bx-file")

	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list = decodeVersionList(t, resp)
	if len(list) != 1 {
		t.Fatalf("want 1 version after first save, got %d", len(list))
	}

	// Save with same content: no new snapshot (documents.version still bumps)
	vBefore := int64(doc["version"].(float64))
	doc = saveDocVersion(t, client, id, vBefore, "Title A", "<p>Content A</p>", "bx-file")
	vAfter := int64(doc["version"].(float64))
	if vAfter != vBefore+1 {
		t.Fatalf("want version bumped to %d, got %d", vBefore+1, vAfter)
	}
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list = decodeVersionList(t, resp)
	if len(list) != 1 {
		t.Fatalf("want still 1 version after identical save, got %d", len(list))
	}

	// Change title only: new snapshot
	doc = saveDocVersion(t, client, id, int64(doc["version"].(float64)), "Title B", "<p>Content A</p>", "bx-file")
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list = decodeVersionList(t, resp)
	if len(list) != 2 {
		t.Fatalf("want 2 versions after title change, got %d", len(list))
	}

	// Change icon only: new snapshot
	doc = saveDocVersion(t, client, id, int64(doc["version"].(float64)), "Title B", "<p>Content A</p>", "bx-folder")
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list = decodeVersionList(t, resp)
	if len(list) != 3 {
		t.Fatalf("want 3 versions after icon change, got %d", len(list))
	}

	// Versions are ordered newest-first
	v0ID := int64(list[0]["id"].(float64))
	v1ID := int64(list[1]["id"].(float64))
	if v0ID <= v1ID {
		t.Fatalf("versions must be ordered newest-first; got ids %d, %d", v0ID, v1ID)
	}

	// GetVersion returns body_html
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions/%d", id, int64(list[1]["id"].(float64))))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get version: want 200, got %d", resp.StatusCode)
	}
	ver := decodeVersion(t, resp)
	if ver["body_html"] == nil || ver["body_html"] == "" {
		t.Fatal("get version: body_html should be present")
	}

	// Restore an older version: returns updated doc, creates a new snapshot
	oldVid := int64(list[2]["id"].(float64)) // oldest = "Title A / Content A / bx-file"
	currentVer := int64(doc["version"].(float64))
	resp = apiPost(t, client, fmt.Sprintf("/api/documents/%d/versions/%d/restore", id, oldVid), map[string]interface{}{
		"version": currentVer,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("restore: want 200, got %d", resp.StatusCode)
	}
	restored := decodeDoc(t, resp)
	if restored["title"] != "Title A" {
		t.Fatalf("restore: want title 'Title A', got %q", restored["title"])
	}
	if restored["icon"] != "bx-file" {
		t.Fatalf("restore: want icon 'bx-file', got %q", restored["icon"])
	}

	// After restore: new snapshot created (4 total)
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list = decodeVersionList(t, resp)
	if len(list) != 4 {
		t.Fatalf("want 4 versions after restore, got %d", len(list))
	}

	// Restore with stale version: 409
	resp = apiPost(t, client, fmt.Sprintf("/api/documents/%d/versions/%d/restore", id, oldVid), map[string]interface{}{
		"version": currentVer, // already used
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale restore: want 409, got %d", resp.StatusCode)
	}

	// GetVersion for non-existent: 404
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions/999999", id))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing version: want 404, got %d", resp.StatusCode)
	}
}

func TestDocumentVersionsRetention(t *testing.T) {
	// Create 3 distinct saves and verify 3 snapshots are retained.
	// (Full pruning test for N<50 requires direct DB access; this validates the happy path.)
	client := loginClient(t)

	resp := apiPost(t, client, "/api/documents", map[string]interface{}{"title": "Retention Doc"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", resp.StatusCode)
	}
	doc := decodeDoc(t, resp)
	id := int64(doc["id"].(float64))

	for i := 1; i <= 3; i++ {
		doc = saveDocVersion(t, client, id, int64(doc["version"].(float64)),
			fmt.Sprintf("Title %d", i),
			fmt.Sprintf("<p>Body %d</p>", i),
			"bx-file",
		)
	}

	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	list := decodeVersionList(t, resp)
	if len(list) != 3 {
		t.Fatalf("want 3 versions after 3 distinct saves, got %d", len(list))
	}
}

func TestDocumentVersionsArchivedBlocked(t *testing.T) {
	client := loginClient(t)

	// Create doc, save one version, then archive it
	resp := apiPost(t, client, "/api/documents", map[string]interface{}{"title": "Archive Test"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", resp.StatusCode)
	}
	doc := decodeDoc(t, resp)
	id := int64(doc["id"].(float64))
	doc = saveDocVersion(t, client, id, int64(doc["version"].(float64)), "Archive Test", "<p>v1</p>", "bx-file")

	resp = apiPost(t, client, fmt.Sprintf("/api/documents/%d/archive", id), map[string]interface{}{})
	resp.Body.Close()

	// Listing versions still works on archived docs
	resp = apiGet(t, client, fmt.Sprintf("/api/documents/%d/versions", id))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list versions on archived doc: want 200, got %d", resp.StatusCode)
	}
	list := decodeVersionList(t, resp)
	resp.Body.Close()
	if len(list) == 0 {
		t.Fatal("want at least 1 version on archived doc")
	}

	// Restore on archived doc: 403
	vid := int64(list[0]["id"].(float64))
	resp = apiPost(t, client, fmt.Sprintf("/api/documents/%d/versions/%d/restore", id, vid), map[string]interface{}{
		"version": int64(doc["version"].(float64)),
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("restore archived: want 403, got %d", resp.StatusCode)
	}
}
