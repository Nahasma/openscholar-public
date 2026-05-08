package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateWriteTarget rejects file paths that contain glob pattern characters.
// Write operations must target a specific file, not a glob expansion.
func validateWriteTarget(path string) error {
	if strings.ContainsAny(path, "*?[]{}") {
		return fmt.Errorf("write target path must not contain glob pattern characters: %s", path)
	}
	return nil
}

func validateArtifactFilename(filename string) error {
	name := strings.TrimSpace(filename)
	if name == "" {
		return fmt.Errorf("filename is required")
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("filename must be a base name without path separators: %s", filename)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("filename must not be %q", name)
	}
	if err := validateWriteTarget(name); err != nil {
		return err
	}
	return nil
}

func validateSearchPattern(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("pattern is required")
	}
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("search pattern must be relative to the search path: %s", pattern)
	}
	parts := strings.FieldsFunc(pattern, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, part := range parts {
		if part == ".." {
			return fmt.Errorf("search pattern must not contain parent-directory traversal: %s", pattern)
		}
	}
	return nil
}
