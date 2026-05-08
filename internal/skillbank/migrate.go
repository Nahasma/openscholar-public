package skillbank

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// migrateLegacySkills moves flat .md files in userDir to {category}/ subdirectories.
//   - Parses frontmatter to get category
//   - Normalizes category (tool-usage → tool_usage)
//   - Invalid/missing category → moves to _inbox/
//   - Name conflicts → moves to _inbox/ (don't overwrite)
//   - Only processes files directly in userDir (not subdirectories)
func migrateLegacySkills(userDir string) {
	files, err := filepath.Glob(filepath.Join(userDir, "*.md"))
	if err != nil || len(files) == 0 {
		return
	}

	for _, f := range files {
		baseName := filepath.Base(f)

		skill, err := ParseSkillFile(f)

		var destCategory string
		if err != nil || skill.Category == "" {
			destCategory = ""
		} else {
			destCategory = normalizeCategory(skill.Category)
			if !validCategories[destCategory] {
				destCategory = ""
			}
		}

		var destDir string
		if destCategory == "" {
			destDir = filepath.Join(userDir, "_inbox")
		} else {
			destDir = filepath.Join(userDir, destCategory)
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			log.Printf("[skillbank] migrate: mkdir %s: %v", destDir, err)
			continue
		}

		destPath := filepath.Join(destDir, baseName)

		// Check for name conflict
		if _, err := os.Stat(destPath); err == nil {
			// Conflict: move to _inbox instead
			inboxPath := filepath.Join(userDir, "_inbox", baseName)
			if err2 := os.MkdirAll(filepath.Join(userDir, "_inbox"), 0o755); err2 == nil {
				if err3 := os.Rename(f, inboxPath); err3 == nil {
					log.Printf("[skillbank] migrate: conflict, moved %s → _inbox/%s", baseName, baseName)
				} else {
					log.Printf("[skillbank] migrate: rename conflict %s to inbox: %v", baseName, err3)
				}
			}
			continue
		}

		if err := os.Rename(f, destPath); err != nil {
			log.Printf("[skillbank] migrate: rename %s → %s: %v", baseName, destPath, err)
			continue
		}

		if destCategory == "" {
			log.Printf("[skillbank] migrate: moved %s → _inbox/ (no valid category)", baseName)
		} else {
			log.Printf("[skillbank] migrate: moved %s → %s/", baseName, destCategory)
		}
	}
}

// normalizeCategory converts category names to canonical form.
// tool-usage → tool_usage (hyphen → underscore)
func normalizeCategory(cat string) string {
	return strings.ReplaceAll(cat, "-", "_")
}

// validCategories defines the allowed category set.
var validCategories = map[string]bool{
	"memory":     true,
	"writing":    true,
	"research":   true,
	"tool_usage": true,
	"domain":     true,
	"workflow":   true,
	"prompt":     true,
	"debug":      true,
}
