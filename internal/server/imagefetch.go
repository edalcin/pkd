package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

// fetchExternalImage downloads rawURL and returns the image bytes, MIME type,
// and a safe filename. Enforces SSRF protection by blocking private/loopback
// IPs at the dialer level (post-DNS), so hostnames resolving to internal
// addresses are also rejected.
func fetchExternalImage(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: ssrfSafeDialer().DialContext,
		},
		// Re-validate each redirect destination via the same dialer.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "PKD/2.0 (+https://github.com/edalcin/pkd)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("http %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	mimeType, _, _ := mime.ParseMediaType(ct)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", "", fmt.Errorf("not an image: content-type %q", ct)
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", "", fmt.Errorf("image exceeds size limit")
	}

	filename := path.Base(u.Path)
	if filename == "" || filename == "." || filename == "/" {
		ext, _ := mime.ExtensionsByType(mimeType)
		if len(ext) > 0 {
			filename = "image" + ext[0]
		} else {
			filename = "image"
		}
	}

	return data, mimeType, filename, nil
}

// ssrfSafeDialer returns a net.Dialer whose Control function rejects connections
// to private, loopback, and link-local IP ranges (including cloud metadata endpoints).
// Uses Go's built-in IP helpers plus explicit CIDRs for ranges not covered by stdlib
// (169.254.0.0/16 metadata, 100.64.0.0/10 CGNAT shared space).
func ssrfSafeDialer() *net.Dialer {
	extraRanges := []string{
		"169.254.0.0/16", // link-local / AWS+GCP metadata endpoint
		"100.64.0.0/10",  // CGNAT shared address space
	}
	var extraNets []*net.IPNet
	for _, cidr := range extraRanges {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil {
			extraNets = append(extraNets, n)
		}
	}

	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("ssrf: bad address: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("ssrf: could not parse ip %q", host)
			}
			// Block via Go stdlib helpers (covers loopback, private, unspecified,
			// link-local unicast/multicast, and all multicast ranges).
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
				ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
				return fmt.Errorf("ssrf: blocked address %s", ip)
			}
			// Block ranges not covered by stdlib helpers.
			for _, n := range extraNets {
				if n.Contains(ip) {
					return fmt.Errorf("ssrf: blocked address %s", ip)
				}
			}
			return nil
		},
	}
}

// isExternalImageSrc returns true if src is an http(s) URL that is not an
// internal attachment (/api/attachments/).
func isExternalImageSrc(src string) bool {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return false
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	return !strings.Contains(u.Path, "/api/attachments/")
}

// extractExternalImageSrcs returns the unique external image src values found
// in the HTML string (http/https, not internal attachments).
func extractExternalImageSrcs(htmlStr string) []string {
	seen := map[string]struct{}{}
	var result []string
	z := html.NewTokenizer(strings.NewReader(htmlStr))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		if tok.Data != "img" {
			continue
		}
		for _, attr := range tok.Attr {
			if attr.Key == "src" && isExternalImageSrc(attr.Val) {
				if _, ok := seen[attr.Val]; !ok {
					seen[attr.Val] = struct{}{}
					result = append(result, attr.Val)
				}
			}
		}
	}
	return result
}

// rewriteImageSrcs replaces external img src values in htmlStr according to
// the mapping (old src → new src). Only exact string replacement is performed;
// the HTML structure is otherwise preserved.
func rewriteImageSrcs(htmlStr string, mapping map[string]string) string {
	result := htmlStr
	for old, new := range mapping {
		// Replace src="<old>" and src='<old>' safely.
		result = strings.ReplaceAll(result, `src="`+old+`"`, `src="`+new+`"`)
		result = strings.ReplaceAll(result, `src='`+old+`'`, `src='`+new+`'`)
	}
	return result
}

// importExternalImages fetches all external images found in htmlStr and saves
// them as attachments for docID. Returns updated HTML and counts of imported
// and failed images. Failures are non-fatal: original src is preserved.
func (s *Server) importExternalImages(ctx context.Context, docID int64, htmlStr string) (newHTML string, imported, failed int) {
	srcs := extractExternalImageSrcs(htmlStr)
	if len(srcs) == 0 {
		return htmlStr, 0, 0
	}
	maxBytes := s.cfg.MaxImageMB * 1024 * 1024
	backend := s.activeStorage()
	mapping := make(map[string]string, len(srcs))
	for _, src := range srcs {
		data, mimeType, filename, err := fetchExternalImage(ctx, src, maxBytes)
		if err != nil {
			failed++
			continue
		}
		att, err := s.attachments.CreateFile(ctx, backend, docID, filename, mimeType, "inline", bytes.NewReader(data), maxBytes)
		if err != nil {
			failed++
			continue
		}
		mapping[src] = att.URL
		imported++
	}
	newHTML = rewriteImageSrcs(htmlStr, mapping)
	return newHTML, imported, failed
}
