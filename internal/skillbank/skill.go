package skillbank

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill represents a general-purpose skill loaded from a .md file.
type Skill struct {
	ID                     string        `yaml:"-" json:"id"`                    // "{category}/{filename}" e.g. "memory/insert"
	Name                   string        `yaml:"name" json:"name"`               // human-readable name
	Description            string        `yaml:"description" json:"description"` // one-line description
	Category               string        `yaml:"category" json:"category"`       // memory | writing | research | tool_usage | domain | workflow | prompt | debug
	Tags                   []string      `yaml:"tags" json:"tags"`               // searchable tags
	Version                int           `yaml:"version" json:"version"`         // auto-incremented on update
	Author                 string        `yaml:"author" json:"author"`           // system | user | agent
	WhenToUse              string        `yaml:"when_to_use,omitempty" json:"when_to_use,omitempty"`
	AllowedTools           []string      `yaml:"allowed-tools,omitempty" json:"allowed_tools,omitempty"`
	Agent                  string        `yaml:"agent,omitempty" json:"agent,omitempty"`
	Context                string        `yaml:"context,omitempty" json:"context,omitempty"`
	Effort                 string        `yaml:"effort,omitempty" json:"effort,omitempty"`
	Model                  string        `yaml:"model,omitempty" json:"model,omitempty"`
	Exposure               SkillExposure `yaml:"exposure,omitempty" json:"exposure,omitempty"`
	UserInvocable          bool          `yaml:"user-invocable,omitempty" json:"user_invocable,omitempty"`
	DisableModelInvocation bool          `yaml:"disable-model-invocation,omitempty" json:"disable_model_invocation,omitempty"`
	Paths                  []string      `yaml:"paths,omitempty" json:"paths,omitempty"`
	Platforms              []string      `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	Source                 string        `yaml:"source,omitempty" json:"source,omitempty"`
	UpdateType             string        `yaml:"update_type,omitempty" json:"update_type,omitempty"` // legacy MemSkill compat: insert/update/delete/noop
	Instruction            string        `yaml:"-" json:"instruction"`                               // body content after frontmatter
	FilePath               string        `yaml:"-" json:"file_path"`                                 // absolute file path on disk
	UsageCount             int           `yaml:"-" json:"usage_count"`                               // from SQLite index
	CreatedAt              time.Time     `yaml:"-" json:"created_at"`
	UpdatedAt              time.Time     `yaml:"-" json:"updated_at"`
}

type SkillExposure string

const (
	SkillExposureImplicit SkillExposure = "implicit"
	SkillExposureExplicit SkillExposure = "explicit"
	SkillExposureBoth     SkillExposure = "both"
	SkillExposureNone     SkillExposure = "none"
)

// QueryOptions defines search parameters for the SkillBank.
type QueryOptions struct {
	Query    string   // FTS5 free-text query
	Category string   // exact category filter
	Tags     []string // any-match tag filter
	Limit    int      // max results (default 5)
}

// ParseSkillFile parses a .md skill file with YAML frontmatter.
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	skill, err := ParseSkillContent("", data)
	if err != nil {
		return nil, fmt.Errorf("%w in %s", err, path)
	}
	skill.FilePath = path
	return skill, nil
}

// ParseSkillContent parses skill content from raw bytes (for embedded FS).
// id is the skill ID to assign; FilePath is not set (bundled skill has no disk path).
func ParseSkillContent(id string, data []byte) (*Skill, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("no frontmatter")
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid frontmatter")
	}

	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(parts[0]), &frontmatter); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	skill := parseSkillFromFrontmatter(frontmatter)

	skill.Instruction = strings.TrimSpace(parts[1])
	skill.ID = id

	// Defaults
	if skill.Version == 0 {
		skill.Version = 1
	}
	if skill.Author == "" {
		skill.Author = "system"
	}

	return &skill, nil
}

func (s Skill) IsUserInvocable() bool {
	if s.Exposure == "" {
		return s.UserInvocable
	}
	return s.Exposure == SkillExposureExplicit || s.Exposure == SkillExposureBoth
}

func (s Skill) IsModelInvocable() bool {
	if s.Exposure == "" {
		return !s.DisableModelInvocation
	}
	return s.Exposure == SkillExposureImplicit || s.Exposure == SkillExposureBoth
}

func (s Skill) IsDisabled() bool {
	if s.Exposure == "" {
		return !s.UserInvocable && s.DisableModelInvocation
	}
	return s.Exposure == SkillExposureNone
}

func ParseSkillExposure(raw string) (SkillExposure, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch SkillExposure(raw) {
	case SkillExposureImplicit, SkillExposureExplicit, SkillExposureBoth, SkillExposureNone:
		return SkillExposure(raw), true
	default:
		return "", false
	}
}

func SkillExposureFromLegacy(userInvocable bool, disableModelInvocation bool) SkillExposure {
	switch {
	case userInvocable && !disableModelInvocation:
		return SkillExposureBoth
	case userInvocable && disableModelInvocation:
		return SkillExposureExplicit
	case !userInvocable && !disableModelInvocation:
		return SkillExposureImplicit
	default:
		return SkillExposureNone
	}
}

// ToMarkdown serializes a Skill back to .md format with YAML frontmatter.
func (s *Skill) ToMarkdown() string {
	var sb strings.Builder
	sb.WriteString("---\n")

	// Build frontmatter manually for clean output
	sb.WriteString(fmt.Sprintf("name: %q\n", s.Name))
	sb.WriteString(fmt.Sprintf("description: %q\n", s.Description))
	sb.WriteString(fmt.Sprintf("category: %q\n", s.Category))

	if len(s.Tags) > 0 {
		sb.WriteString("tags: [")
		for i, t := range s.Tags {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", t))
		}
		sb.WriteString("]\n")
	}

	sb.WriteString(fmt.Sprintf("version: %d\n", s.Version))
	sb.WriteString(fmt.Sprintf("author: %q\n", s.Author))
	if s.WhenToUse != "" {
		sb.WriteString(fmt.Sprintf("when_to_use: %q\n", s.WhenToUse))
	}
	if len(s.AllowedTools) > 0 {
		sb.WriteString("allowed-tools: [")
		for i, tool := range s.AllowedTools {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", tool))
		}
		sb.WriteString("]\n")
	}
	if s.Agent != "" {
		sb.WriteString(fmt.Sprintf("agent: %q\n", s.Agent))
	}
	if s.Context != "" {
		sb.WriteString(fmt.Sprintf("context: %q\n", s.Context))
	}
	if s.Effort != "" {
		sb.WriteString(fmt.Sprintf("effort: %q\n", s.Effort))
	}
	if s.Model != "" {
		sb.WriteString(fmt.Sprintf("model: %q\n", s.Model))
	}
	if s.Exposure != "" {
		sb.WriteString(fmt.Sprintf("exposure: %q\n", s.Exposure))
	}
	// Keep legacy booleans for compatibility.
	sb.WriteString(fmt.Sprintf("user-invocable: %t\n", s.IsUserInvocable()))
	sb.WriteString(fmt.Sprintf("disable-model-invocation: %t\n", !s.IsModelInvocable()))
	if len(s.Paths) > 0 {
		sb.WriteString("paths: [")
		for i, p := range s.Paths {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", p))
		}
		sb.WriteString("]\n")
	}
	if len(s.Platforms) > 0 {
		sb.WriteString("platforms: [")
		for i, p := range s.Platforms {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", p))
		}
		sb.WriteString("]\n")
	}
	if s.Source != "" {
		sb.WriteString(fmt.Sprintf("source: %q\n", s.Source))
	}

	if s.UpdateType != "" {
		sb.WriteString(fmt.Sprintf("update_type: %s\n", s.UpdateType))
	}

	sb.WriteString("---\n")
	sb.WriteString(s.Instruction)
	sb.WriteString("\n")

	return sb.String()
}

func (s Skill) metadataJSON() string {
	meta := map[string]any{}
	if s.WhenToUse != "" {
		meta["when_to_use"] = s.WhenToUse
	}
	if len(s.AllowedTools) > 0 {
		meta["allowed-tools"] = s.AllowedTools
	}
	if s.Agent != "" {
		meta["agent"] = s.Agent
	}
	if s.Context != "" {
		meta["context"] = s.Context
	}
	if s.Effort != "" {
		meta["effort"] = s.Effort
	}
	if s.Model != "" {
		meta["model"] = s.Model
	}
	if s.Exposure != "" {
		meta["exposure"] = s.Exposure
		meta["user-invocable"] = s.IsUserInvocable()
		meta["disable-model-invocation"] = !s.IsModelInvocable()
	} else {
		if s.UserInvocable {
			meta["user-invocable"] = true
		}
		if s.DisableModelInvocation {
			meta["disable-model-invocation"] = true
		}
	}
	if len(s.Paths) > 0 {
		meta["paths"] = s.Paths
	}
	if len(s.Platforms) > 0 {
		meta["platforms"] = s.Platforms
	}
	if s.Source != "" {
		meta["source"] = s.Source
	}
	if len(meta) == 0 {
		return "{}"
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s Skill) searchDocument() string {
	parts := []string{
		s.WhenToUse,
		strings.Join(s.AllowedTools, " "),
		s.Agent,
		s.Context,
		s.Effort,
		s.Model,
		strings.Join(s.Paths, " "),
		strings.Join(s.Platforms, " "),
		s.Source,
		s.Instruction,
	}
	filtered := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, "\n\n")
}

func applyMetadataJSON(s *Skill, raw string) {
	if raw == "" || raw == "{}" {
		return
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return
	}
	if s.WhenToUse == "" {
		s.WhenToUse = asString(meta, "when_to_use", "when-to-use")
	}
	if len(s.AllowedTools) == 0 {
		s.AllowedTools = asStringSlice(meta, "allowed-tools", "allowed_tools")
	}
	if s.Agent == "" {
		s.Agent = asString(meta, "agent")
	}
	if s.Context == "" {
		s.Context = asString(meta, "context")
	}
	if s.Effort == "" {
		s.Effort = asString(meta, "effort")
	}
	if s.Model == "" {
		s.Model = asString(meta, "model")
	}
	if s.Exposure == "" {
		if exp, ok := asExposure(meta, "exposure"); ok {
			s.Exposure = exp
		} else {
			s.Exposure = exposureFromLegacyBools(meta)
		}
	}
	if s.Exposure != "" {
		s.UserInvocable = s.IsUserInvocable()
		s.DisableModelInvocation = !s.IsModelInvocable()
	} else {
		if !s.UserInvocable {
			s.UserInvocable = asBool(meta, "user-invocable", "user_invocable")
		}
		if !s.DisableModelInvocation {
			s.DisableModelInvocation = asBool(meta, "disable-model-invocation", "disable_model_invocation")
		}
	}
	if len(s.Paths) == 0 {
		s.Paths = asStringSlice(meta, "paths")
	}
	if len(s.Platforms) == 0 {
		s.Platforms = asStringSlice(meta, "platforms")
	}
	if s.Source == "" {
		s.Source = asString(meta, "source")
	}
}

func parseSkillFromFrontmatter(frontmatter map[string]any) Skill {
	var s Skill
	s.Name = asString(frontmatter, "name")
	s.Description = asString(frontmatter, "description")
	s.Category = asString(frontmatter, "category")
	s.Tags = asStringSlice(frontmatter, "tags")
	s.Version = asInt(frontmatter, "version")
	s.Author = asString(frontmatter, "author")
	s.UpdateType = asString(frontmatter, "update_type", "update-type")
	s.WhenToUse = asString(frontmatter, "when_to_use", "when-to-use")
	s.AllowedTools = asStringSlice(frontmatter, "allowed-tools", "allowed_tools")
	s.Agent = asString(frontmatter, "agent")
	s.Context = asString(frontmatter, "context")
	s.Effort = asString(frontmatter, "effort")
	s.Model = asString(frontmatter, "model")
	if exp, ok := asExposure(frontmatter, "exposure"); ok {
		s.Exposure = exp
	} else {
		s.Exposure = exposureFromLegacyBools(frontmatter)
	}
	s.UserInvocable = s.IsUserInvocable()
	s.DisableModelInvocation = !s.IsModelInvocable()
	s.Paths = asStringSlice(frontmatter, "paths")
	s.Platforms = asStringSlice(frontmatter, "platforms")
	s.Source = asString(frontmatter, "source")
	return s
}

func asString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch tv := v.(type) {
		case string:
			return strings.TrimSpace(tv)
		}
	}
	return ""
}

func asStringSlice(m map[string]any, keys ...string) []string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch tv := v.(type) {
		case []any:
			out := make([]string, 0, len(tv))
			for _, item := range tv {
				if s, ok := item.(string); ok && s != "" {
					out = append(out, s)
				}
			}
			return out
		case []string:
			out := make([]string, 0, len(tv))
			for _, s := range tv {
				if s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func asInt(m map[string]any, keys ...string) int {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch tv := v.(type) {
		case int:
			return tv
		case int64:
			return int(tv)
		case float64:
			return int(tv)
		}
	}
	return 0
}

func asBool(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func asBoolWithPresence(m map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

func asExposure(m map[string]any, keys ...string) (SkillExposure, bool) {
	return ParseSkillExposure(asString(m, keys...))
}

func exposureFromLegacyBools(frontmatter map[string]any) SkillExposure {
	userInvocable, hasUser := asBoolWithPresence(frontmatter, "user-invocable", "user_invocable")
	disableModel, hasDisable := asBoolWithPresence(frontmatter, "disable-model-invocation", "disable_model_invocation")
	if !hasUser && !hasDisable {
		return ""
	}
	return SkillExposureFromLegacy(userInvocable, disableModel)
}
