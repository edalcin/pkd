package integration_test

import (
	"net/http"
	"net/url"
	"sync"
)

// cookieJar is a minimal in-memory cookie jar for integration tests.
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string][]*http.Cookie // keyed by host
}

func newJar() *cookieJar { return &cookieJar{cookies: make(map[string][]*http.Cookie)} }

func (j *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}

func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cookies[u.Host]
}

// csrfFromJar extracts the pkd_csrf cookie value for the given base URL.
func csrfFromJar(jar *cookieJar, baseURL string) string {
	u, _ := url.Parse(baseURL)
	for _, c := range jar.Cookies(u) {
		if c.Name == "pkd_csrf" {
			return c.Value
		}
	}
	return ""
}
