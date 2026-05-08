package debug

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RotateLogs removes old session directories and legacy JSONL files from logDir.
// Session directories and JSONL files older than maxAge are deleted.
// It is safe to call even if logDir does not exist.
func RotateLogs(logDir string, maxAge time.Duration) error {
	if logDir == "" {
		return nil
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		fullPath := filepath.Join(logDir, entry.Name())

		// Clean up old session directories
		if entry.IsDir() {
			os.RemoveAll(fullPath)
			continue
		}

		// Legacy: clean up flat .jsonl files
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			os.Remove(fullPath)
		}
	}
	return nil
}
