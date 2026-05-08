package fileop

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidateBoundary checks that targetPath is within baseDir.
// If baseDir is empty, validation is skipped.
// Relative paths are resolved against baseDir.
func ValidateBoundary(targetPath, baseDir string) error {
	if baseDir == "" {
		return nil
	}
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(baseDir, targetPath)
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	absBase := filepath.Clean(baseDir)
	if absTarget == absBase || strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("path %s is outside boundary %s", absTarget, absBase)
}

var traversalPattern = regexp.MustCompile(`(?:^|[/\\])\.\.[/\\]`)

// ContainsTraversal detects path traversal patterns (../).
func ContainsTraversal(path string) bool {
	return traversalPattern.MatchString(path) || strings.HasSuffix(path, "/..")
}

// ResolveRelative resolves a relative path against baseDir.
// Absolute paths are returned unchanged.
func ResolveRelative(path, baseDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
