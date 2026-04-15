package security

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrPathTraversal is returned when a stored filename would escape the base directory.
var ErrPathTraversal = errors.New("path traversal attempt detected")

// SafeAttachmentPath resolves the full path for a stored attachment filename
// and verifies it is contained within base. Returns ErrPathTraversal if the
// resolved path would escape base or if storedFilename contains suspicious
// components such as "..", absolute paths, or null bytes.
func SafeAttachmentPath(base, storedFilename string) (string, error) {
	if storedFilename == "" {
		return "", ErrPathTraversal
	}
	// Reject null bytes
	if strings.ContainsRune(storedFilename, 0) {
		return "", ErrPathTraversal
	}
	// Reject absolute paths (both Unix '/' prefix and Windows drive letters)
	if filepath.IsAbs(storedFilename) ||
		strings.HasPrefix(storedFilename, "/") ||
		strings.HasPrefix(storedFilename, "\\") {
		return "", ErrPathTraversal
	}
	// Reject ".." components before cleaning
	for _, part := range strings.FieldsFunc(storedFilename, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." || part == "." {
			return "", ErrPathTraversal
		}
	}

	clean := filepath.Clean(filepath.Join(base, storedFilename))
	cleanBase := filepath.Clean(base)

	if !strings.HasPrefix(clean, cleanBase+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	return clean, nil
}
