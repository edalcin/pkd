// Package backup builds and consumes attachment backup archives.
//
// Backup ZIPs are backend-agnostic: each entry is keyed by the SHA256 of the
// attachment content, and a manifest.json (last entry in the archive) maps
// each SHA256 back to the storage filenames recorded in the attachments
// table. This layout lets a backup produced against any storage backend
// (local filesystem, S3) be restored on any other.
package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"
)

// ManifestFileName is the fixed name of the manifest entry inside the ZIP.
const ManifestFileName = "manifest.json"

// CurrentManifestVersion is the schema version this build produces.
const CurrentManifestVersion = 1

// Manifest describes the contents of a backup archive.
type Manifest struct {
	Version           int             `json:"version"`
	CreatedAt         time.Time       `json:"created_at"`
	SourceEnvironment EnvDescriptor   `json:"source_environment"`
	Entries           []ManifestEntry `json:"entries"`
}

// EnvDescriptor records (for auditability) the environment that produced the
// archive. Restore does not depend on these fields.
type EnvDescriptor struct {
	BackendKind string `json:"backend_kind"`
	Bucket      string `json:"bucket,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
}

// ManifestEntry maps a content hash to the storage filenames that point at it.
// A single entry may correspond to multiple attachment rows when the same
// content is referenced more than once (natural deduplication).
type ManifestEntry struct {
	SHA256          string   `json:"sha256"`
	SizeBytes       int64    `json:"size_bytes"`
	MimeType        string   `json:"mime_type,omitempty"`
	StoredFilenames []string `json:"stored_filenames"`
}

var sha256HexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate ensures m is well-formed and the version is supported.
func (m *Manifest) Validate() error {
	if m.Version != CurrentManifestVersion {
		return fmt.Errorf("manifest version %d not supported (expected %d)", m.Version, CurrentManifestVersion)
	}
	for i, e := range m.Entries {
		if !sha256HexRE.MatchString(e.SHA256) {
			return fmt.Errorf("entry %d: invalid sha256 %q", i, e.SHA256)
		}
		if e.SizeBytes < 0 {
			return fmt.Errorf("entry %d: negative size", i)
		}
		if len(e.StoredFilenames) == 0 {
			return fmt.Errorf("entry %d: empty stored_filenames", i)
		}
	}
	return nil
}

// Encode writes m as indented JSON.
func (m *Manifest) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// DecodeManifest reads a Manifest from r and validates it.
func DecodeManifest(r io.Reader) (*Manifest, error) {
	var m Manifest
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
