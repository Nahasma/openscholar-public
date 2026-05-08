package research

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ResearchRunPolicy struct {
	AutoProceed        bool   `json:"auto_proceed"`
	HumanCheckpoint    bool   `json:"human_checkpoint"`
	Effort             string `json:"effort"`
	Assurance          string `json:"assurance"`
	ReviewerDifficulty string `json:"reviewer_difficulty"`
	AutoWrite          bool   `json:"auto_write"`
	Venue              string `json:"venue,omitempty"`
	MaxReviewRounds    int    `json:"max_review_rounds"`
	BatchPolicy        string `json:"batch_policy"`
	TraceMode          string `json:"trace"`
}

type OutputManifestEntry struct {
	TimestampUTC time.Time `json:"timestamp_utc,omitempty"`
	Phase        string    `json:"phase"`
	ArtifactPath string    `json:"artifact_path"`
	Note         string    `json:"note,omitempty"`
}

func IsARISTemplate(template string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(template)), "aris")
}

func DefaultResearchRunPolicy() ResearchRunPolicy {
	return ResearchRunPolicy{
		AutoProceed:        true,
		HumanCheckpoint:    false,
		Effort:             "balanced",
		Assurance:          "draft",
		ReviewerDifficulty: "medium",
		AutoWrite:          false,
		MaxReviewRounds:    4,
		BatchPolicy:        "auto",
		TraceMode:          "full",
	}
}

func NormalizeResearchRunPolicy(in ResearchRunPolicy) ResearchRunPolicy {
	p := DefaultResearchRunPolicy()
	if in.AutoProceed {
		p.AutoProceed = true
	}
	if in.HumanCheckpoint {
		p.HumanCheckpoint = true
	}
	if strings.TrimSpace(in.Effort) != "" {
		p.Effort = strings.ToLower(strings.TrimSpace(in.Effort))
	}
	if strings.TrimSpace(in.Assurance) != "" {
		p.Assurance = strings.ToLower(strings.TrimSpace(in.Assurance))
	}
	if strings.TrimSpace(in.ReviewerDifficulty) != "" {
		p.ReviewerDifficulty = strings.ToLower(strings.TrimSpace(in.ReviewerDifficulty))
	}
	if in.AutoWrite {
		p.AutoWrite = true
	}
	if strings.TrimSpace(in.Venue) != "" {
		p.Venue = strings.TrimSpace(in.Venue)
	}
	if in.MaxReviewRounds > 0 {
		p.MaxReviewRounds = in.MaxReviewRounds
	}
	if strings.TrimSpace(in.BatchPolicy) != "" {
		p.BatchPolicy = strings.ToLower(strings.TrimSpace(in.BatchPolicy))
	}
	if strings.TrimSpace(in.TraceMode) != "" {
		p.TraceMode = strings.ToLower(strings.TrimSpace(in.TraceMode))
	}
	if p.Assurance == "" {
		if p.Effort == "max" || p.Effort == "beast" {
			p.Assurance = "submission"
		} else {
			p.Assurance = "draft"
		}
	}
	if (p.Effort == "max" || p.Effort == "beast") && strings.TrimSpace(in.Assurance) == "" {
		p.Assurance = "submission"
	}
	if p.HumanCheckpoint {
		p.AutoProceed = false
	}
	return p
}

func InitARISWorkspace(workDir string, policy ResearchRunPolicy) error {
	root, err := filepath.Abs(workDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	dirs := []string{
		".aris",
		".aris/traces",
		"idea-stage",
		"refine-logs",
		"experiments",
		"experiments/nodes",
		"review-stage",
		"research-wiki",
		".handoff",
		"paper",
		"paper/sections",
		"code",
	}
	for _, d := range dirs {
		if err := mkdirAllNoSymlink(root, d); err != nil {
			return fmt.Errorf("init aris workspace dir %s: %w", d, err)
		}
	}

	policy = NormalizeResearchRunPolicy(policy)
	runCfgPath, err := prepareArtifactTarget(workDir, ".aris/run_config.json")
	if err != nil {
		return fmt.Errorf("prepare run config: %w", err)
	}
	body, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run policy: %w", err)
	}
	if err := os.WriteFile(runCfgPath, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write run config: %w", err)
	}
	return AppendOutputManifest(workDir, OutputManifestEntry{
		Phase:        "init",
		ArtifactPath: ".aris/run_config.json",
		Note:         "ok",
	})
}

func WriteVersionedArtifact(workDir, relPath string, content []byte, now time.Time) (string, string, error) {
	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return "", "", fmt.Errorf("invalid relative artifact path: %s", relPath)
	}
	ext := filepath.Ext(cleanRel)
	base := strings.TrimSuffix(cleanRel, ext)
	ts := now.UTC().Format("20060102T150405Z")
	versionedRel := base + "." + ts + ext

	versionedAbs, err := prepareArtifactTarget(workDir, versionedRel)
	if err != nil {
		return "", "", err
	}
	latestAbs, err := prepareArtifactTarget(workDir, cleanRel)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(versionedAbs, content, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(latestAbs, content, 0o644); err != nil {
		return "", "", err
	}
	return versionedRel, cleanRel, nil
}

func prepareArtifactTarget(workDir, relPath string) (string, error) {
	root, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	abs := filepath.Join(root, relPath)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("artifact path escapes workspace: %s", relPath)
	}
	if err := mkdirAllNoSymlink(root, filepath.Dir(rel)); err != nil {
		return "", err
	}
	if info, err := os.Lstat(abs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("artifact target is a symlink: %s", relPath)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return abs, nil
}

func mkdirAllNoSymlink(root, relDir string) error {
	cleanDir := filepath.Clean(relDir)
	if cleanDir == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(cleanDir, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
					return err
				}
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path contains symlink directory: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("artifact path component is not a directory: %s", current)
		}
	}
	return nil
}

func AppendOutputManifest(workDir string, entry OutputManifestEntry) error {
	path, err := prepareArtifactTarget(workDir, "MANIFEST.md")
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := "| timestamp_utc | phase | artifact | note |\n| --- | --- | --- | --- |\n"
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			return err
		}
	}
	ts := entry.TimestampUTC
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	line := fmt.Sprintf("| %s | %s | %s | %s |\n",
		ts.UTC().Format(time.RFC3339),
		sanitizeManifestCell(entry.Phase),
		sanitizeManifestCell(entry.ArtifactPath),
		sanitizeManifestCell(entry.Note),
	)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func sanitizeManifestCell(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "|", "/")
}
