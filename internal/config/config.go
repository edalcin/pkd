package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// Required
	Password        string
	DBPath          string
	AttachmentsPath string

	// Optional with defaults
	ListenAddr          string
	SessionIdleMinutes  int
	MaxImageMB          int64
	MaxAttachmentMB     int64
	TrustProxyHeaders   bool

	// BaseURL is the public-facing base URL used when generating share links
	// (e.g. "https://pkd.dalc.in/"). If empty, the server derives it from the
	// incoming request's Host header, which may be incorrect behind a proxy.
	BaseURL string

	// ImportToken is the pre-shared secret for the /api/import endpoint used by
	// external apps (e.g. notas). If empty, the endpoint is disabled.
	ImportToken string
}

// Load reads configuration from environment variables. It returns an error
// if any required variable is missing or any value is invalid.
func Load() (*Config, error) {
	var errs []string

	password := os.Getenv("PKD_PASSWORD")
	if password == "" {
		errs = append(errs, "PKD_PASSWORD is required")
	}

	dbPath := os.Getenv("PKD_DB_PATH")
	if dbPath == "" {
		errs = append(errs, "PKD_DB_PATH is required")
	}

	attachmentsPath := os.Getenv("PKD_ATTACHMENTS_PATH")
	if attachmentsPath == "" {
		errs = append(errs, "PKD_ATTACHMENTS_PATH is required")
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	cfg := &Config{
		Password:        password,
		DBPath:          dbPath,
		AttachmentsPath: attachmentsPath,
		ListenAddr:      ":8080",
		SessionIdleMinutes: 60,
		MaxImageMB:      10,
		MaxAttachmentMB: 100,
	}

	if v := os.Getenv("PKD_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	if v := os.Getenv("PKD_SESSION_IDLE_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PKD_SESSION_IDLE_MINUTES must be a positive integer, got %q", v)
		}
		cfg.SessionIdleMinutes = n
	}

	if v := os.Getenv("PKD_MAX_IMAGE_MB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PKD_MAX_IMAGE_MB must be a positive integer, got %q", v)
		}
		cfg.MaxImageMB = n
	}

	if v := os.Getenv("PKD_MAX_ATTACHMENT_MB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PKD_MAX_ATTACHMENT_MB must be a positive integer, got %q", v)
		}
		cfg.MaxAttachmentMB = n
	}

	if v := os.Getenv("PKD_TRUST_PROXY_HEADERS"); v == "1" || v == "true" || v == "yes" {
		cfg.TrustProxyHeaders = true
	}

	if v := os.Getenv("PKD_BASE_URL"); v != "" {
		// Ensure it ends with a trailing slash for consistent URL construction
		cfg.BaseURL = strings.TrimRight(v, "/") + "/"
	}

	if v := os.Getenv("PKD_IMPORT_TOKEN"); v != "" {
		cfg.ImportToken = v
	}

	return cfg, nil
}
