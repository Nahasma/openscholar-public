package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

type skillManageTool struct {
	skills skillbank.Service
}

type skillManageParams struct {
	Action                 string    `json:"action"`                // create | update | delete | import | install | sync | uninstall | list | info
	ID                     string    `json:"id,omitempty"`          // required for update/delete
	Name                   string    `json:"name,omitempty"`        // required for create
	Description            string    `json:"description,omitempty"` // required for create
	Category               string    `json:"category,omitempty"`    // required for create
	Tags                   []string  `json:"tags,omitempty"`
	Instruction            string    `json:"instruction,omitempty"`              // skill body content
	WhenToUse              *string   `json:"when_to_use,omitempty"`              // update patch field (supports clearing)
	AllowedTools           *[]string `json:"allowed_tools,omitempty"`            // update patch field (supports clearing)
	Agent                  *string   `json:"agent,omitempty"`                    // update patch field (supports clearing)
	Context                *string   `json:"context,omitempty"`                  // inline | fork
	Effort                 *string   `json:"effort,omitempty"`                   // update patch field (supports clearing)
	Model                  *string   `json:"model,omitempty"`                    // update patch field (supports clearing)
	Exposure               *string   `json:"exposure,omitempty"`                 // implicit | explicit | both | none
	UserInvocable          *bool     `json:"user_invocable,omitempty"`           // update patch field (supports clearing)
	DisableModelInvocation *bool     `json:"disable_model_invocation,omitempty"` // update patch field (supports clearing)
	Paths                  *[]string `json:"paths,omitempty"`                    // update patch field (supports clearing)
	Platforms              *[]string `json:"platforms,omitempty"`                // update patch field (supports clearing)
	Source                 *string   `json:"source,omitempty"`                   // update patch field (supports clearing)
	Dir                    string    `json:"dir,omitempty"`                      // directory path for import
	LocalPath              string    `json:"local_path,omitempty"`               // local markdown path for install
}

// NewSkillManageTool creates a deferred tool for managing skills in the skill bank.
func NewSkillManageTool(skills skillbank.Service) BaseTool {
	return &skillManageTool{skills: skills}
}

func (t *skillManageTool) Info() ToolInfo {
	return ToolInfo{
		Name: "SkillManage",
		Description: "Create, update, or delete skills in the skill bank. " +
			"Use to save new patterns, refine existing skills based on experience, or remove outdated skills.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "update", "delete", "import", "install", "sync", "uninstall", "list", "info"},
					"description": "The operation to perform (import: batch import .md files; install: managed local install from file or bundle directory; sync/uninstall/list/info: governance operations)",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "Skill ID (e.g., 'writing/apa_citation'). Required for update and delete.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (required for create)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "One-line skill description (required for create)",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "Skill category: memory, writing, research, tool_usage, domain, workflow, prompt, debug",
				},
				"tags": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Searchable tags",
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "Skill body content (When to use / How to apply / Examples / Constraints)",
				},
				"when_to_use": map[string]any{
					"type":        "string",
					"description": "When to use this skill (update supports clearing with empty string).",
				},
				"allowed_tools": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Allowed tool list for this skill (update supports clearing with []).",
				},
				"agent": map[string]any{
					"type":        "string",
					"description": "Preferred agent alias for this skill (update supports clearing with empty string).",
				},
				"context": map[string]any{
					"type":        "string",
					"enum":        []string{"inline", "fork"},
					"description": "Execution context hint for InvokeSkill. Use fork for self-contained sub-agent runs.",
				},
				"effort": map[string]any{
					"type":        "string",
					"description": "Effort hint (e.g. low/medium/high).",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Preferred model alias or ID.",
				},
				"exposure": map[string]any{
					"type":        "string",
					"enum":        []string{"implicit", "explicit", "both", "none"},
					"description": "Skill exposure: implicit=model only, explicit=user slash only, both=user and model, none=disabled.",
				},
				"user_invocable": map[string]any{
					"type":        "boolean",
					"description": "Whether this skill should be slash-command invocable.",
				},
				"disable_model_invocation": map[string]any{
					"type":        "boolean",
					"description": "Whether model auto-invocation should be disabled for this skill.",
				},
				"paths": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Path globs used for conditional activation.",
				},
				"platforms": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Platform hints for this skill.",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Skill source label.",
				},
				"dir": map[string]any{
					"type":        "string",
					"description": "Directory path containing .md skill files (for import action). Defaults to data/skills/_inbox.",
				},
				"local_path": map[string]any{
					"type":        "string",
					"description": "Local path for managed install. Supports a skill markdown file (category/name.md or category/name/SKILL.md) or a bundle directory containing SKILL.md.",
				},
			},
			"required": []string{"action"},
		},
		Required: []string{"action"},
	}
}

func (t *skillManageTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params skillManageParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	// Block write operations in research mode; allow read-only actions.
	if IsResearchMode(ctx) && (params.Action == "create" || params.Action == "update" || params.Action == "delete" || params.Action == "import" || params.Action == "install" || params.Action == "sync" || params.Action == "uninstall") {
		return NewTextErrorResponse("Research mode is active. Skill modification is not allowed in research mode. Use Shift+Tab to switch to default mode."), nil
	}

	switch params.Action {
	case "create":
		if params.Name == "" || params.Category == "" || params.Instruction == "" {
			return NewTextErrorResponse("create requires: name, category, instruction"), nil
		}
		category, ok := normalizeSkillManageCategory(params.Category)
		if !ok {
			return NewTextErrorResponse(fmt.Sprintf("invalid category %q (use one of: memory, writing, research, tool_usage, domain, workflow, prompt, debug)", params.Category)), nil
		}
		if safeName := sanitizeSkillIDName(params.Name); safeName != "" {
			candidateID := category + "/" + safeName
			if _, err := t.skills.Get(ctx, candidateID); err == nil {
				return NewTextErrorResponse(fmt.Sprintf("Skill '%s' already exists. Use action=update with id '%s' instead of create.", candidateID, candidateID)), nil
			}
		}
		skill := skillbank.Skill{
			Name:        params.Name,
			Description: params.Description,
			Category:    category,
			Tags:        params.Tags,
			Author:      "agent",
			Instruction: params.Instruction,
		}
		if params.WhenToUse != nil {
			skill.WhenToUse = *params.WhenToUse
		}
		if params.AllowedTools != nil {
			skill.AllowedTools = *params.AllowedTools
		}
		if params.Agent != nil {
			skill.Agent = *params.Agent
		}
		if params.Context != nil {
			skill.Context = *params.Context
		}
		if params.Effort != nil {
			skill.Effort = *params.Effort
		}
		if params.Model != nil {
			skill.Model = *params.Model
		}
		if params.Exposure != nil {
			exposure, ok := skillbank.ParseSkillExposure(*params.Exposure)
			if !ok {
				return NewTextErrorResponse("invalid exposure (use one of: implicit, explicit, both, none)"), nil
			}
			skill.Exposure = exposure
			skill.UserInvocable = skill.IsUserInvocable()
			skill.DisableModelInvocation = !skill.IsModelInvocable()
		}
		if params.UserInvocable != nil {
			skill.UserInvocable = *params.UserInvocable
		}
		if params.DisableModelInvocation != nil {
			skill.DisableModelInvocation = *params.DisableModelInvocation
		}
		if params.Exposure == nil && (params.UserInvocable != nil || params.DisableModelInvocation != nil) {
			skill.Exposure = skillbank.SkillExposureFromLegacy(skill.UserInvocable, skill.DisableModelInvocation)
		}
		if params.Paths != nil {
			skill.Paths = *params.Paths
		}
		if params.Platforms != nil {
			skill.Platforms = *params.Platforms
		}
		if params.Source != nil {
			skill.Source = *params.Source
		}
		if err := t.skills.Create(ctx, skill); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to create skill: %v", err)), nil
		}
		return NewTextResponse(fmt.Sprintf("Skill '%s' created in category '%s'.", params.Name, category)), nil

	case "update":
		if params.ID == "" {
			return NewTextErrorResponse("update requires: id"), nil
		}

		existing, err := t.skills.Get(ctx, params.ID)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to load skill for update: %v", err)), nil
		}

		updated := *existing
		if params.Name != "" {
			updated.Name = params.Name
		}
		if params.Description != "" {
			updated.Description = params.Description
		}
		if params.Tags != nil {
			updated.Tags = params.Tags
		}
		if params.Instruction != "" {
			updated.Instruction = params.Instruction
		}
		if params.WhenToUse != nil {
			updated.WhenToUse = *params.WhenToUse
		}
		if params.AllowedTools != nil {
			updated.AllowedTools = *params.AllowedTools
		}
		if params.Agent != nil {
			updated.Agent = *params.Agent
		}
		if params.Context != nil {
			updated.Context = *params.Context
		}
		if params.Effort != nil {
			updated.Effort = *params.Effort
		}
		if params.Model != nil {
			updated.Model = *params.Model
		}
		if params.Exposure != nil {
			exposure, ok := skillbank.ParseSkillExposure(*params.Exposure)
			if !ok {
				return NewTextErrorResponse("invalid exposure (use one of: implicit, explicit, both, none)"), nil
			}
			updated.Exposure = exposure
			updated.UserInvocable = updated.IsUserInvocable()
			updated.DisableModelInvocation = !updated.IsModelInvocable()
		}
		if params.UserInvocable != nil {
			updated.UserInvocable = *params.UserInvocable
		}
		if params.DisableModelInvocation != nil {
			updated.DisableModelInvocation = *params.DisableModelInvocation
		}
		if params.Exposure == nil && (params.UserInvocable != nil || params.DisableModelInvocation != nil) {
			updated.Exposure = skillbank.SkillExposureFromLegacy(updated.UserInvocable, updated.DisableModelInvocation)
		}
		if params.Paths != nil {
			updated.Paths = *params.Paths
		}
		if params.Platforms != nil {
			updated.Platforms = *params.Platforms
		}
		if params.Source != nil {
			updated.Source = *params.Source
		}
		if err := t.skills.Update(ctx, params.ID, updated); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to update skill: %v", err)), nil
		}
		return NewTextResponse(fmt.Sprintf("Skill '%s' updated (version incremented).", params.ID)), nil

	case "delete":
		if params.ID == "" {
			return NewTextErrorResponse("delete requires: id"), nil
		}
		if err := t.skills.Delete(ctx, params.ID); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to delete skill: %v", err)), nil
		}
		return NewTextResponse(fmt.Sprintf("Skill '%s' deleted.", params.ID)), nil

	case "import":
		dir := params.Dir
		// Empty dir means use service's default inbox (inside .openscholar/skills/_inbox)
		imported, errs := t.skills.ImportFromDir(ctx, dir)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Imported %d skill(s) from %s.", imported, dir)
		for _, e := range errs {
			fmt.Fprintf(&sb, "\n  Error: %v", e)
		}
		return NewTextResponse(sb.String()), nil

	case "install":
		gov, ok := t.skills.(skillbank.GovernanceService)
		if !ok {
			return NewTextErrorResponse("Skill governance is not available for install."), nil
		}
		if strings.TrimSpace(params.LocalPath) == "" {
			return NewTextErrorResponse("install requires: local_path"), nil
		}
		res, err := gov.Install(ctx, skillbank.InstallOptions{LocalPath: params.LocalPath})
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to install skill: %v", err)), nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Installed skill '%s'.", res.SkillID)
		if res.Status != "" {
			fmt.Fprintf(&sb, "\nStatus: %s", res.Status)
		}
		if res.InstalledPath != "" {
			fmt.Fprintf(&sb, "\nManaged path: %s", res.InstalledPath)
		}
		if res.ManifestPath != "" {
			fmt.Fprintf(&sb, "\nManifest: %s", res.ManifestPath)
		}
		return NewTextResponse(sb.String()), nil
	case "sync":
		gov, ok := t.skills.(skillbank.GovernanceService)
		if !ok {
			return NewTextErrorResponse("Skill governance is not available for sync."), nil
		}
		if strings.TrimSpace(params.ID) == "" {
			return NewTextErrorResponse("sync requires: id"), nil
		}
		res, err := gov.Sync(ctx, skillbank.SyncOptions{ID: params.ID})
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to sync skill: %v", err)), nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Synced skill '%s'.", res.SkillID)
		if res.Status != "" {
			fmt.Fprintf(&sb, "\nStatus: %s", res.Status)
		}
		if res.InstalledPath != "" {
			fmt.Fprintf(&sb, "\nManaged path: %s", res.InstalledPath)
		}
		if res.ManifestPath != "" {
			fmt.Fprintf(&sb, "\nManifest: %s", res.ManifestPath)
		}
		return NewTextResponse(sb.String()), nil
	case "uninstall":
		gov, ok := t.skills.(skillbank.GovernanceService)
		if !ok {
			return NewTextErrorResponse("Skill governance is not available for uninstall."), nil
		}
		if strings.TrimSpace(params.ID) == "" {
			return NewTextErrorResponse("uninstall requires: id"), nil
		}
		res, err := gov.Uninstall(ctx, skillbank.UninstallOptions{ID: params.ID})
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to uninstall skill: %v", err)), nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Uninstalled skill '%s'.", res.SkillID)
		if res.Status != "" {
			fmt.Fprintf(&sb, "\nStatus: %s", res.Status)
		}
		if res.RemovedPath != "" {
			fmt.Fprintf(&sb, "\nRemoved path: %s", res.RemovedPath)
		}
		if res.ManifestPath != "" {
			fmt.Fprintf(&sb, "\nManifest: %s", res.ManifestPath)
		}
		return NewTextResponse(sb.String()), nil

	case "list":
		gov, ok := t.skills.(skillbank.GovernanceService)
		if !ok {
			return NewTextErrorResponse("Skill governance is not available for list."), nil
		}
		list, err := gov.ListGoverned(ctx, strings.TrimSpace(params.Category))
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to list governed skills: %v", err)), nil
		}
		return NewTextResponse(formatGovernedSkillList(list, strings.TrimSpace(params.Category))), nil

	case "info":
		gov, ok := t.skills.(skillbank.GovernanceService)
		if !ok {
			return NewTextErrorResponse("Skill governance is not available for info."), nil
		}
		if strings.TrimSpace(params.ID) == "" {
			return NewTextErrorResponse("info requires: id"), nil
		}
		info, err := gov.Info(ctx, params.ID)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to inspect skill governance: %v", err)), nil
		}
		return NewTextResponse(formatGovernedSkillInfo(info)), nil

	default:
		return NewTextErrorResponse(fmt.Sprintf("Unknown action: %s (use create, update, delete, import, install, sync, uninstall, list, or info)", params.Action)), nil
	}
}

func sanitizeSkillIDName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, name)
	return name
}

func normalizeSkillManageCategory(category string) (string, bool) {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.ReplaceAll(category, "-", "_")
	switch category {
	case "memory", "writing", "research", "tool_usage", "domain", "workflow", "prompt", "debug":
		return category, true
	default:
		return "", false
	}
}

func formatGovernedSkillList(list []skillbank.GovernedSkill, category string) string {
	if len(list) == 0 {
		if category != "" {
			return fmt.Sprintf("No governed skills found in category '%s'.", category)
		}
		return "No governed skills found."
	}

	var sb strings.Builder
	if category != "" {
		fmt.Fprintf(&sb, "Governed skills in '%s' (%d):\n", category, len(list))
	} else {
		fmt.Fprintf(&sb, "Governed skills (%d):\n", len(list))
	}
	for _, item := range list {
		active := "none"
		if item.ActiveSource != nil {
			active = fmt.Sprintf("%s/%s", item.ActiveSource.SourceTier, item.ActiveSource.SourceKind)
			if trust := sourceTrustLabel(*item.ActiveSource); trust != "" {
				active += " (" + trust + ")"
			}
		}
		fmt.Fprintf(&sb, "\n- %s — active=%s sources=%d", item.Meta.ID, active, len(item.Sources))
		if item.Meta.Description != "" {
			fmt.Fprintf(&sb, "\n  %s", item.Meta.Description)
		}
	}
	return sb.String()
}

func formatGovernedSkillInfo(info *skillbank.GovernedSkill) string {
	if info == nil {
		return "No governance info found."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s [%s]\n", info.Meta.Name, info.Meta.ID)
	if info.Meta.Description != "" {
		fmt.Fprintf(&sb, "%s\n", info.Meta.Description)
	}
	if info.ActiveSource != nil {
		fmt.Fprintf(&sb, "\nActive source: %s / %s\n", info.ActiveSource.SourceTier, info.ActiveSource.SourceKind)
		if trust := sourceTrustLabel(*info.ActiveSource); trust != "" {
			fmt.Fprintf(&sb, "Trust: %s\n", trust)
		}
		if info.ActiveSource.SourceKey != "" {
			fmt.Fprintf(&sb, "Source key: %s\n", info.ActiveSource.SourceKey)
		}
		if info.ActiveSource.FilePath != "" {
			fmt.Fprintf(&sb, "File path: %s\n", info.ActiveSource.FilePath)
		}
	}
	if info.Meta.Source != "" {
		fmt.Fprintf(&sb, "Declared source: %s\n", info.Meta.Source)
	}
	if len(info.Sources) > 0 {
		sb.WriteString("\nSources:\n")
		for _, src := range info.Sources {
			state := "shadowed"
			if src.IsActive {
				state = "active"
			}
			fmt.Fprintf(&sb, "- %s [%s/%s]\n", state, src.SourceTier, src.SourceKind)
			if trust := sourceTrustLabel(src); trust != "" {
				fmt.Fprintf(&sb, "  trust: %s\n", trust)
			}
			if src.SourceKey != "" {
				fmt.Fprintf(&sb, "  key: %s\n", src.SourceKey)
			}
			if src.FilePath != "" {
				fmt.Fprintf(&sb, "  path: %s\n", src.FilePath)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

func sourceTrustLabel(src skillbank.SkillSource) string {
	if src.Meta == nil {
		return ""
	}
	trust, _ := src.Meta["trust_level"].(string)
	scan, _ := src.Meta["scan_status"].(string)
	if trust == "" && scan == "" {
		return ""
	}
	if trust == "" {
		return scan
	}
	if scan == "" {
		return trust
	}
	return trust + "/" + scan
}
