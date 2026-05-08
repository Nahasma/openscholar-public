package skillbank

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// PathsActivator activates skills whose `paths` metadata match recently touched files.
// Activation is represented by usage increments so list/search ranking can learn from
// contextual path touches even before the skill is explicitly invoked.
type PathsActivator struct {
	skills Service
}

func NewPathsActivator(skills Service) *PathsActivator {
	return &PathsActivator{skills: skills}
}

// ActivateByPaths matches skill metadata `paths` globs against provided file paths.
// Returns the number of skills activated (usage incremented).
func (a *PathsActivator) ActivateByPaths(ctx context.Context, workspace string, paths []string) (int, error) {
	if a == nil || a.skills == nil || len(paths) == 0 {
		return 0, nil
	}

	metas, err := a.skills.ListMetadata(ctx)
	if err != nil {
		return 0, err
	}

	candidates := pathCandidates(workspace, paths)
	if len(candidates) == 0 {
		return 0, nil
	}

	activated := 0
	for _, meta := range metas {
		if len(meta.Paths) == 0 {
			continue
		}
		if !matchesAnyPath(meta.Paths, candidates) {
			continue
		}
		if err := a.skills.RecordUsage(ctx, meta.ID, true); err == nil {
			activated++
		}
	}
	return activated, nil
}

func matchesAnyPath(patterns []string, candidates []string) bool {
	for _, rawPattern := range patterns {
		pattern := normalizePathForMatch(rawPattern)
		if pattern == "" {
			continue
		}
		for _, cand := range candidates {
			if ok, _ := doublestar.Match(pattern, cand); ok {
				return true
			}
		}
	}
	return false
}

func pathCandidates(workspace string, paths []string) []string {
	out := make([]string, 0, len(paths)*3)
	seen := make(map[string]struct{}, len(paths)*3)
	workspace = filepath.Clean(workspace)
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		addCandidate(seen, &out, normalizePathForMatch(raw))

		if filepath.IsAbs(raw) && workspace != "" {
			if rel, err := filepath.Rel(workspace, raw); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				addCandidate(seen, &out, normalizePathForMatch(rel))
			}
		}

		if !filepath.IsAbs(raw) && workspace != "" {
			addCandidate(seen, &out, normalizePathForMatch(filepath.Join(workspace, raw)))
		}
	}
	return out
}

func addCandidate(seen map[string]struct{}, out *[]string, candidate string) {
	if candidate == "" {
		return
	}
	if _, ok := seen[candidate]; ok {
		return
	}
	seen[candidate] = struct{}{}
	*out = append(*out, candidate)
}

func normalizePathForMatch(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "./")
}
