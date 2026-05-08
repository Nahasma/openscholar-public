package skillbank

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type scannedSkill struct {
	Skill      Skill
	Relative   string
	SourcePath string
}

// scanUserDir scans all .md files under userDir, organized by category subdirectories.
// Supports both:
//   - userDir/{category}/{name}.md
//   - userDir/{category}/{name}/SKILL.md
//
// Skips _inbox and dot-prefixed directories.
func scanUserDir(userDir string) ([]Skill, error) {
	scanned, err := scanUserDirDetailed(userDir)
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(scanned))
	for _, sk := range scanned {
		skills = append(skills, sk.Skill)
	}
	return skills, nil
}

func scanUserDirDetailed(userDir string) ([]scannedSkill, error) {
	skillByID := make(map[string]Skill)
	metaByID := make(map[string]scannedSkill)
	bundleByID := make(map[string]bool)

	entries, err := os.ReadDir(userDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		// Skip _inbox, _managed and dot-directories
		if dirName == "_inbox" || dirName == "_managed" || strings.HasPrefix(dirName, ".") {
			continue
		}

		category := normalizeCategory(dirName)
		categoryDir := filepath.Join(userDir, dirName)

		// Legacy layout: {category}/{name}.md
		legacyFiles, err := filepath.Glob(filepath.Join(categoryDir, "*.md"))
		if err == nil {
			for _, f := range legacyFiles {
				absPath, err := filepath.Abs(f)
				if err != nil {
					absPath = f
				}

				skill, err := ParseSkillFile(absPath)
				if err != nil {
					continue
				}

				name := strings.TrimSuffix(filepath.Base(f), ".md")
				id := category + "/" + name
				if bundleByID[id] {
					continue
				}
				skill.ID = id
				skill.Category = category
				skill.FilePath = absPath
				skillByID[id] = *skill
				metaByID[id] = scannedSkill{
					Skill:      *skill,
					Relative:   filepath.ToSlash(filepath.Join(dirName, filepath.Base(f))),
					SourcePath: absPath,
				}
			}
		}

		// Bundle layout: {category}/{name}/SKILL.md
		nameEntries, err := os.ReadDir(categoryDir)
		if err != nil {
			continue
		}
		for _, nameEntry := range nameEntries {
			if !nameEntry.IsDir() {
				continue
			}
			nameDir := nameEntry.Name()
			if nameDir == "_inbox" || strings.HasPrefix(nameDir, ".") {
				continue
			}

			skillPath := filepath.Join(categoryDir, nameDir, "SKILL.md")
			absPath, err := filepath.Abs(skillPath)
			if err != nil {
				absPath = skillPath
			}

			skill, err := ParseSkillFile(absPath)
			if err != nil {
				continue
			}

			id := category + "/" + nameDir
			skill.ID = id
			skill.Category = category
			skill.FilePath = absPath
			skillByID[id] = *skill
			bundleByID[id] = true
			metaByID[id] = scannedSkill{
				Skill:      *skill,
				Relative:   filepath.ToSlash(filepath.Join(dirName, nameDir, "SKILL.md")),
				SourcePath: absPath,
			}
		}
	}

	ids := make([]string, 0, len(skillByID))
	for id := range skillByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	skills := make([]scannedSkill, 0, len(ids))
	for _, id := range ids {
		if sk, ok := metaByID[id]; ok {
			skills = append(skills, sk)
			continue
		}
		skills = append(skills, scannedSkill{
			Skill:      skillByID[id],
			SourcePath: skillByID[id].FilePath,
		})
	}

	return skills, nil
}

// scanBundledFS scans embedded system skills from fs.FS.
// Supports both:
//   - {category}/{name}.md
//   - {category}/{name}/SKILL.md
//
// Skips _inbox and dot-prefixed directories.
func scanBundledFS(fsys fs.FS) ([]Skill, error) {
	scanned, err := scanBundledFSDetailed(fsys)
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(scanned))
	for _, sk := range scanned {
		skills = append(skills, sk.Skill)
	}
	return skills, nil
}

func scanBundledFSDetailed(fsys fs.FS) ([]scannedSkill, error) {
	skillByID := make(map[string]Skill)
	metaByID := make(map[string]scannedSkill)
	bundleByID := make(map[string]bool)

	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.IsDir() {
			name := d.Name()
			// Skip root
			if path == "." {
				return nil
			}
			// Skip _inbox and dot-directories
			if name == "_inbox" || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		parts := strings.Split(path, "/")
		var category, name string
		isBundle := false
		switch {
		// Legacy layout: {category}/{name}.md
		case len(parts) == 2 && strings.HasSuffix(parts[1], ".md"):
			category = normalizeCategory(parts[0])
			name = strings.TrimSuffix(parts[1], ".md")
		// Bundle layout: {category}/{name}/SKILL.md
		case len(parts) == 3 && parts[2] == "SKILL.md":
			category = normalizeCategory(parts[0])
			name = parts[1]
			isBundle = true
		default:
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil
		}

		id := category + "/" + name
		if bundleByID[id] && !isBundle {
			return nil
		}
		skill, err := ParseSkillContent(id, data)
		if err != nil {
			return nil
		}

		skill.Author = "system"
		skill.Category = category

		skillByID[id] = *skill
		bundleByID[id] = isBundle
		metaByID[id] = scannedSkill{
			Skill:      *skill,
			Relative:   filepath.ToSlash(path),
			SourcePath: filepath.ToSlash(path),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(skillByID))
	for id := range skillByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	skills := make([]scannedSkill, 0, len(ids))
	for _, id := range ids {
		if sk, ok := metaByID[id]; ok {
			skills = append(skills, sk)
			continue
		}
		skills = append(skills, scannedSkill{
			Skill:      skillByID[id],
			SourcePath: skillByID[id].FilePath,
		})
	}

	return skills, nil
}

func scanManagedDir(managedDir string) ([]scannedSkill, error) {
	skillByID := make(map[string]Skill)
	metaByID := make(map[string]scannedSkill)
	bundleByID := make(map[string]bool)

	entries, err := os.ReadDir(managedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if strings.HasPrefix(dirName, ".") {
			continue
		}
		category := normalizeCategory(dirName)
		categoryDir := filepath.Join(managedDir, dirName)

		legacyFiles, err := filepath.Glob(filepath.Join(categoryDir, "*.md"))
		if err == nil {
			for _, f := range legacyFiles {
				absPath, err := filepath.Abs(f)
				if err != nil {
					absPath = f
				}
				skill, err := ParseSkillFile(absPath)
				if err != nil {
					continue
				}
				name := strings.TrimSuffix(filepath.Base(f), ".md")
				id := category + "/" + name
				if bundleByID[id] {
					continue
				}
				skill.ID = id
				skill.Category = category
				skill.FilePath = absPath
				skillByID[id] = *skill
				metaByID[id] = scannedSkill{
					Skill:      *skill,
					Relative:   filepath.ToSlash(filepath.Join(dirName, filepath.Base(f))),
					SourcePath: absPath,
				}
			}
		}

		nameEntries, err := os.ReadDir(categoryDir)
		if err != nil {
			continue
		}
		for _, nameEntry := range nameEntries {
			if !nameEntry.IsDir() {
				continue
			}
			nameDir := nameEntry.Name()
			if strings.HasPrefix(nameDir, ".") {
				continue
			}

			skillPath := filepath.Join(categoryDir, nameDir, "SKILL.md")
			absPath, err := filepath.Abs(skillPath)
			if err != nil {
				absPath = skillPath
			}
			skill, err := ParseSkillFile(absPath)
			if err != nil {
				continue
			}

			id := category + "/" + nameDir
			skill.ID = id
			skill.Category = category
			skill.FilePath = absPath
			skillByID[id] = *skill
			bundleByID[id] = true
			metaByID[id] = scannedSkill{
				Skill:      *skill,
				Relative:   filepath.ToSlash(filepath.Join(dirName, nameDir, "SKILL.md")),
				SourcePath: absPath,
			}
		}
	}

	ids := make([]string, 0, len(skillByID))
	for id := range skillByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]scannedSkill, 0, len(ids))
	for _, id := range ids {
		if sk, ok := metaByID[id]; ok {
			out = append(out, sk)
			continue
		}
		out = append(out, scannedSkill{
			Skill:      skillByID[id],
			SourcePath: skillByID[id].FilePath,
		})
	}
	return out, nil
}
