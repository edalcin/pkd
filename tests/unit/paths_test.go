package unit_test

import (
	"testing"

	"github.com/edalcin/pkd/internal/security"
)

func TestSafeAttachmentPath_Valid(t *testing.T) {
	base := "/data/attachments"
	cases := []string{
		"ab/cd/abcdef",
		"12/34/12345678",
	}
	for _, stored := range cases {
		path, err := security.SafeAttachmentPath(base, stored)
		if err != nil {
			t.Errorf("expected valid path for %q, got error: %v", stored, err)
		}
		if path == "" {
			t.Errorf("expected non-empty path for %q", stored)
		}
	}
}

func TestSafeAttachmentPath_Traversal(t *testing.T) {
	base := "/data/attachments"
	malicious := []string{
		"../etc/passwd",
		"../../root/.ssh/id_rsa",
		"ab/../../../etc/passwd",
		"/etc/passwd",
		"ab/cd/../../etc/passwd",
		"ab\x00cd",          // null byte
		"",                   // empty
		"ab/../../escape",
	}
	for _, stored := range malicious {
		_, err := security.SafeAttachmentPath(base, stored)
		if err == nil {
			t.Errorf("expected error (path traversal) for %q, got nil", stored)
		}
	}
}
