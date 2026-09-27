package integration_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// shareTargetPost sends POST /api/capture the way Android's share sheet does:
// a form post with the session cookie and no X-CSRF-Token header.
func shareTargetPost(t *testing.T, client *http.Client, fetchSite string) *http.Response {
	t.Helper()
	form := url.Values{"title": {"Mata Atlântica"}, "text": {"https://pt.wikipedia.org/wiki/Mata_Atl%C3%A2ntica"}}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/capture", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", fetchSite)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestShareTarget_BrowserInitiated_RedirectsToNota(t *testing.T) {
	client := loginClient(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp := shareTargetPost(t, client, "none")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/#/doc/") {
		t.Fatalf("want Location /#/doc/<id>, got %q", loc)
	}
}

func TestShareTarget_CrossSite_StillNeedsCSRFToken(t *testing.T) {
	client := loginClient(t)
	if resp := shareTargetPost(t, client, "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}
