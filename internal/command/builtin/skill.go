package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

type skillImportCmd struct{}

func (c *skillImportCmd) Name() string { return "skill-import" }
func (c *skillImportCmd) Description() string {
	return "Import skills from a directory into the skill bank"
}

func (c *skillImportCmd) Execute(ctx command.Context) command.Result {
	dir := strings.TrimSpace(ctx.Args)
	// Empty dir means use service's default inbox

	if ctx.App.SkillBank == nil {
		return command.Result{Output: "SkillBank not initialized."}
	}

	imported, errs := ctx.App.SkillBank.ImportFromDir(context.Background(), dir)

	var sb strings.Builder
	if imported > 0 {
		fmt.Fprintf(&sb, "Imported %d skill(s) from %s.\n", imported, dir)
	} else {
		fmt.Fprintf(&sb, "No skills found in %s.\n", dir)
	}
	for _, e := range errs {
		fmt.Fprintf(&sb, "  Error: %v\n", e)
	}

	return command.Result{Output: sb.String()}
}

type skillListCmd struct{}

func (c *skillListCmd) Name() string        { return "skill-list" }
func (c *skillListCmd) Description() string { return "List skills in the skill bank" }

func (c *skillListCmd) Execute(ctx command.Context) command.Result {
	if ctx.App.SkillBank == nil {
		return command.Result{Output: "SkillBank not initialized."}
	}

	category := strings.TrimSpace(ctx.Args)
	skills, err := ctx.App.SkillBank.List(context.Background(), category)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Error: %v", err)}
	}

	if len(skills) == 0 {
		msg := "No skills found."
		if category != "" {
			msg = fmt.Sprintf("No skills found in category '%s'.", category)
		}
		return command.Result{Output: msg}
	}

	var sb strings.Builder
	if category != "" {
		fmt.Fprintf(&sb, "Skills in '%s' (%d):\n", category, len(skills))
	} else {
		fmt.Fprintf(&sb, "All skills (%d):\n", len(skills))
	}

	currentCat := ""
	for _, s := range skills {
		if s.Category != currentCat {
			currentCat = s.Category
			fmt.Fprintf(&sb, "\n  [%s]\n", currentCat)
		}
		tags := ""
		if len(s.Tags) > 0 {
			tags = " (" + strings.Join(s.Tags, ", ") + ")"
		}
		fmt.Fprintf(&sb, "    %-24s %s%s\n", s.ID, s.Description, tags)
	}

	return command.Result{Output: sb.String()}
}

type skillInstallCmd struct{}

func (c *skillInstallCmd) Name() string { return "skill-install" }
func (c *skillInstallCmd) Description() string {
	return "Install a managed skill from a local markdown path or bundle directory"
}

func (c *skillInstallCmd) Execute(ctx command.Context) command.Result {
	gov, ok := governedSkillBank(ctx)
	if !ok {
		return command.Result{Output: "Skill governance is not available."}
	}

	localPath := strings.TrimSpace(ctx.Args)
	if localPath == "" {
		return command.Result{Output: "Usage: /skill-install <local-path>"}
	}

	res, err := gov.Install(context.Background(), skillbank.InstallOptions{LocalPath: localPath})
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Install failed: %v", err)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Installed skill '%s'.\n", res.SkillID)
	if res.Status != "" {
		fmt.Fprintf(&sb, "Status: %s\n", res.Status)
	}
	if res.InstalledPath != "" {
		fmt.Fprintf(&sb, "Managed path: %s\n", res.InstalledPath)
	}
	if res.ManifestPath != "" {
		fmt.Fprintf(&sb, "Manifest: %s\n", res.ManifestPath)
	}
	return command.Result{Output: sb.String()}
}

type skillSyncCmd struct{}

func (c *skillSyncCmd) Name() string { return "skill-sync" }
func (c *skillSyncCmd) Description() string {
	return "Sync a managed skill from its recorded local source"
}

func (c *skillSyncCmd) Execute(ctx command.Context) command.Result {
	gov, ok := governedSkillBank(ctx)
	if !ok {
		return command.Result{Output: "Skill governance is not available."}
	}

	id := strings.TrimSpace(ctx.Args)
	if id == "" {
		return command.Result{Output: "Usage: /skill-sync <skill-id>"}
	}

	res, err := gov.Sync(context.Background(), skillbank.SyncOptions{ID: id})
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Sync failed: %v", err)}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Synced skill '%s'.\n", res.SkillID)
	if res.Status != "" {
		fmt.Fprintf(&sb, "Status: %s\n", res.Status)
	}
	if res.InstalledPath != "" {
		fmt.Fprintf(&sb, "Managed path: %s\n", res.InstalledPath)
	}
	if res.ManifestPath != "" {
		fmt.Fprintf(&sb, "Manifest: %s\n", res.ManifestPath)
	}
	return command.Result{Output: sb.String()}
}

type skillUninstallCmd struct{}

func (c *skillUninstallCmd) Name() string { return "skill-uninstall" }
func (c *skillUninstallCmd) Description() string {
	return "Uninstall a managed skill by ID"
}

func (c *skillUninstallCmd) Execute(ctx command.Context) command.Result {
	gov, ok := governedSkillBank(ctx)
	if !ok {
		return command.Result{Output: "Skill governance is not available."}
	}

	id := strings.TrimSpace(ctx.Args)
	if id == "" {
		return command.Result{Output: "Usage: /skill-uninstall <skill-id>"}
	}

	res, err := gov.Uninstall(context.Background(), skillbank.UninstallOptions{ID: id})
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Uninstall failed: %v", err)}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Uninstalled skill '%s'.\n", res.SkillID)
	if res.Status != "" {
		fmt.Fprintf(&sb, "Status: %s\n", res.Status)
	}
	if res.RemovedPath != "" {
		fmt.Fprintf(&sb, "Removed path: %s\n", res.RemovedPath)
	}
	if res.ManifestPath != "" {
		fmt.Fprintf(&sb, "Manifest: %s\n", res.ManifestPath)
	}
	return command.Result{Output: sb.String()}
}

type skillInfoCmd struct{}

func (c *skillInfoCmd) Name() string        { return "skill-info" }
func (c *skillInfoCmd) Description() string { return "Show governance info for a skill" }

func (c *skillInfoCmd) Execute(ctx command.Context) command.Result {
	gov, ok := governedSkillBank(ctx)
	if !ok {
		return command.Result{Output: "Skill governance is not available."}
	}

	id := strings.TrimSpace(ctx.Args)
	if id == "" {
		return command.Result{Output: "Usage: /skill-info <skill-id>"}
	}

	info, err := gov.Info(context.Background(), id)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Error: %v", err)}
	}
	return command.Result{Output: formatGovernedSkillCommandInfo(info)}
}

type skillInstalledCmd struct{}

func (c *skillInstalledCmd) Name() string { return "skill-installed" }
func (c *skillInstalledCmd) Description() string {
	return "List governed installed skills and active provenance"
}

func (c *skillInstalledCmd) Execute(ctx command.Context) command.Result {
	gov, ok := governedSkillBank(ctx)
	if !ok {
		return command.Result{Output: "Skill governance is not available."}
	}

	category := strings.TrimSpace(ctx.Args)
	list, err := gov.ListGoverned(context.Background(), category)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Error: %v", err)}
	}
	return command.Result{Output: formatGovernedSkillCommandList(list, category)}
}

func governedSkillBank(ctx command.Context) (skillbank.GovernanceService, bool) {
	if ctx.App == nil || ctx.App.SkillBank == nil {
		return nil, false
	}
	gov, ok := ctx.App.SkillBank.(skillbank.GovernanceService)
	return gov, ok
}

func formatGovernedSkillCommandList(list []skillbank.GovernedSkill, category string) string {
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
			if trust := commandSourceTrustLabel(*item.ActiveSource); trust != "" {
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

func formatGovernedSkillCommandInfo(info *skillbank.GovernedSkill) string {
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
		if trust := commandSourceTrustLabel(*info.ActiveSource); trust != "" {
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
			if trust := commandSourceTrustLabel(src); trust != "" {
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

func commandSourceTrustLabel(src skillbank.SkillSource) string {
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
