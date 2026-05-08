package hooks

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Condition determines whether a hook should fire for a given input.
type Condition interface {
	Match(input Input) bool
}

// alwaysCondition matches all inputs (no condition configured).
type alwaysCondition struct{}

func (alwaysCondition) Match(_ Input) bool { return true }

// ToolCondition matches when the input's ToolName satisfies the configured glob.
// An empty pattern or "*" matches everything.
type ToolCondition struct {
	Pattern string
}

// Match returns true when input.ToolName matches the pattern.
// Supports exact matches and the "*" wildcard.
func (c ToolCondition) Match(input Input) bool {
	return matchStringPattern(c.Pattern, input.ToolName)
}

// PathCondition matches when any path value found in input.ToolInput satisfies
// the configured glob pattern. It reuses the doublestar library in the same way
// as permission.MatchFileRule.
type PathCondition struct {
	Pattern string
}

// MetadataCondition matches structured runtime metadata such as provider,
// source, error_kind, progress_kind, and goal_type.
type MetadataCondition struct {
	Field   string
	Pattern string
}

func (c MetadataCondition) Match(input Input) bool {
	var value string
	switch c.Field {
	case "provider":
		value = input.Provider
	case "source":
		value = input.Source
	case "error_kind":
		value = input.ErrorKind
	case "progress_kind":
		value = input.ProgressKind
	case "goal_type":
		value = input.GoalType
	default:
		return false
	}
	return matchStringPattern(c.Pattern, value)
}

// ArtifactCondition matches artifact paths reported by tool metadata.
type ArtifactCondition struct {
	Pattern string
}

func (c ArtifactCondition) Match(input Input) bool {
	if c.Pattern == "" || c.Pattern == "*" || c.Pattern == "**" {
		return true
	}
	for _, path := range input.ArtifactPaths {
		if matchGlobPath(c.Pattern, path) {
			return true
		}
	}
	return false
}

func matchStringPattern(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return true
	}
	if matched, err := doublestar.Match(strings.ToLower(pattern), strings.ToLower(value)); err == nil && matched {
		return true
	}
	return strings.EqualFold(value, pattern)
}

// pathKeys are the ToolInput keys that hold file path values.
// Only these keys are checked for glob matching, avoiding false positives
// from non-path fields like "prompt" or "content".
var pathKeys = []string{"file_path", "path", "command", "directory"}

// Match returns true when at least one path value in input.ToolInput matches
// the glob pattern. Only well-known path-bearing keys are checked.
func (c PathCondition) Match(input Input) bool {
	if c.Pattern == "" || c.Pattern == "*" || c.Pattern == "**" {
		return true
	}
	for _, key := range pathKeys {
		v, ok := input.ToolInput[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if matchGlobPath(c.Pattern, s) {
			return true
		}
	}
	return false
}

// matchGlobPath performs case-normalised doublestar glob matching on a file
// path, mirroring the logic in permission.MatchFileRule.
func matchGlobPath(pattern, filePath string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		pattern = strings.ToLower(pattern)
		filePath = strings.ToLower(filePath)
	}
	filePath = filepath.ToSlash(filePath)
	pattern = filepath.ToSlash(pattern)

	matched, err := doublestar.Match(pattern, filePath)
	if err != nil {
		return false
	}
	if matched {
		return true
	}
	// Also try the basename alone (e.g. "*.go" should match "/foo/bar.go").
	base := filepath.Base(filePath)
	matched, _ = doublestar.Match(pattern, base)
	return matched
}

// andCondition combines two conditions with logical AND.
type andCondition struct {
	a, b Condition
}

func (c andCondition) Match(input Input) bool {
	return c.a.Match(input) && c.b.Match(input)
}

// buildCondition creates a Condition from an IfConfig.
// Returns alwaysCondition when cfg is nil.
func buildCondition(cfg *IfConfig) Condition {
	if cfg == nil {
		return alwaysCondition{}
	}

	var conds []Condition
	if cfg.Tool != "" {
		conds = append(conds, ToolCondition{Pattern: cfg.Tool})
	}
	if cfg.Path != "" {
		conds = append(conds, PathCondition{Pattern: cfg.Path})
	}
	if cfg.Provider != "" {
		conds = append(conds, MetadataCondition{Field: "provider", Pattern: cfg.Provider})
	}
	if cfg.Source != "" {
		conds = append(conds, MetadataCondition{Field: "source", Pattern: cfg.Source})
	}
	if cfg.ErrorKind != "" {
		conds = append(conds, MetadataCondition{Field: "error_kind", Pattern: cfg.ErrorKind})
	}
	if cfg.ProgressKind != "" {
		conds = append(conds, MetadataCondition{Field: "progress_kind", Pattern: cfg.ProgressKind})
	}
	if cfg.GoalType != "" {
		conds = append(conds, MetadataCondition{Field: "goal_type", Pattern: cfg.GoalType})
	}
	if cfg.Artifact != "" {
		conds = append(conds, ArtifactCondition{Pattern: cfg.Artifact})
	}

	switch len(conds) {
	case 0:
		return alwaysCondition{}
	case 1:
		return conds[0]
	default:
		// Combine all with AND.
		result := Condition(conds[0])
		for _, c := range conds[1:] {
			result = andCondition{a: result, b: c}
		}
		return result
	}
}
