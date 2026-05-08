package fileop

import (
	"fmt"
	"os"
	"path/filepath"
)

// SafeWrite combines path validation + directory creation + atomic write.
// Options control behavior (boundary check, mkdir, permissions, atomic mode).
func SafeWrite(path string, data []byte, opts ...Option) error {
	cfg := &writeConfig{perm: 0o644, atomic: true}
	for _, o := range opts {
		o(cfg)
	}

	if ContainsTraversal(path) {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	if cfg.boundary != "" {
		if err := ValidateBoundary(path, cfg.boundary); err != nil {
			return err
		}
	}

	if cfg.mkdir {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}

	if cfg.atomic {
		return WriteFileAtomic(path, data, cfg.perm)
	}
	return os.WriteFile(path, data, cfg.perm)
}

type writeConfig struct {
	perm     os.FileMode
	boundary string
	mkdir    bool
	atomic   bool
}

// Option configures SafeWrite behavior.
type Option func(*writeConfig)

// WithBoundary enables boundary validation against baseDir.
func WithBoundary(baseDir string) Option { return func(c *writeConfig) { c.boundary = baseDir } }

// WithMkdir creates parent directories before writing.
func WithMkdir() Option { return func(c *writeConfig) { c.mkdir = true } }

// WithPerm sets the file permission (default 0644).
func WithPerm(perm os.FileMode) Option { return func(c *writeConfig) { c.perm = perm } }

// WithNonAtomic disables atomic write (for testing or temporary files).
func WithNonAtomic() Option { return func(c *writeConfig) { c.atomic = false } }
