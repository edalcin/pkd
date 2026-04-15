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

	return cfg, nil
}
