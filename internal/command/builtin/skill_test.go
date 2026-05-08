package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/skillbank"
)

type fakeGovernedSkillBank struct {
	installResult   *skillbank.InstallResult
	installErr      error
	syncResult      *skillbank.SyncResult
	syncErr         error
	uninstallResult *skillbank.UninstallResult
	uninstallErr    error
	listResult      []skillbank.GovernedSkill
	listErr         error
	infoResult      *skillbank.GovernedSkill
	infoErr         error
	installedPath   string
}

func (f *fakeGovernedSkillBank) Search(context.Context, skillbank.QueryOptions) ([]skillbank.Skill, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) SearchMetadata(context.Context, skillbank.QueryOptions) ([]skillbank.SkillSearchHit, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) View(context.Context, string) (*skillbank.Skill, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) Get(context.Context, string) (*skillbank.Skill, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) List(context.Context, string) ([]skillbank.Skill, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) ListMetadata(context.Context) ([]skillbank.SkillMeta, error) {
	return nil, nil
}
func (f *fakeGovernedSkillBank) Create(context.Context, skillbank.Skill) error         { return nil }
func (f *fakeGovernedSkillBank) Update(context.Context, string, skillbank.Skill) error { return nil }
func (f *fakeGovernedSkillBank) Delete(context.Context, string) error                  { return nil }
func (f *fakeGovernedSkillBank) RecordUsage(context.Context, string, bool) error       { return nil }
func (f *fakeGovernedSkillBank) ImportFromDir(context.Context, string) (int, []error)  { return 0, nil }
func (f *fakeGovernedSkillBank) Reindex(context.Context) error                         { return nil }
func (f *fakeGovernedSkillBank) SetLLMCaller(skillbank.LLMCaller)                      {}

func (f *fakeGovernedSkillBank) Install(context.Context, skillbank.InstallOptions) (*skillbank.InstallResult, error) {
	return f.installResult, f.installErr
}
func (f *fakeGovernedSkillBank) Sync(context.Context, skillbank.SyncOptions) (*skillbank.SyncResult, error) {
	return f.syncResult, f.syncErr
}
func (f *fakeGovernedSkillBank) Uninstall(context.Context, skillbank.UninstallOptions) (*skillbank.UninstallResult, error) {
	return f.uninstallResult, f.uninstallErr
}
func (f *fakeGovernedSkillBank) ListGoverned(context.Context, string) ([]skillbank.GovernedSkill, error) {
	return f.listResult, f.listErr
}
func (f *fakeGovernedSkillBank) Info(context.Context, string) (*skillbank.GovernedSkill, error) {
	return f.infoResult, f.infoErr
}

func TestSkillInstallCmd(t *testing.T) {
	bank := &fakeGovernedSkillBank{
		installResult: &skillbank.InstallResult{
			SkillID:       "workflow/sample",
			Status:        "installed",
			InstalledPath: "/tmp/.openscholar/skills/_managed/workflow/sample/SKILL.md",
			ManifestPath:  "/tmp/.openscholar/skills/_managed/manifest.json",
		},
	}

	res := (&skillInstallCmd{}).Execute(command.Context{
		App:  &app.App{SkillBank: bank},
		Args: "/tmp/workflow/sample.md",
	})
	if !strings.Contains(res.Output, "Installed skill 'workflow/sample'.") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Status: installed") {
		t.Fatalf("expected install status in output: %s", res.Output)
	}
}

func TestSkillInstalledCmd(t *testing.T) {
	bank := &fakeGovernedSkillBank{
		listResult: []skillbank.GovernedSkill{
			{
				Meta: skillbank.SkillMeta{ID: "workflow/sample", Description: "sample desc"},
				ActiveSource: &skillbank.SkillSource{
					SourceTier: "user-managed",
					SourceKind: "local-file",
				},
				Sources: []skillbank.SkillSource{{ID: "active"}},
			},
		},
	}

	res := (&skillInstalledCmd{}).Execute(command.Context{
		App:  &app.App{SkillBank: bank},
		Args: "workflow",
	})
	if !strings.Contains(res.Output, "workflow/sample") || !strings.Contains(res.Output, "user-managed/local-file") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestSkillSyncCmd(t *testing.T) {
	bank := &fakeGovernedSkillBank{
		syncResult: &skillbank.SyncResult{
			SkillID:       "workflow/sample",
			Status:        "synced",
			InstalledPath: "/tmp/.openscholar/skills/_managed/workflow/sample/SKILL.md",
			ManifestPath:  "/tmp/.openscholar/skills/_managed/manifest.json",
		},
	}
	res := (&skillSyncCmd{}).Execute(command.Context{
		App:  &app.App{SkillBank: bank},
		Args: "workflow/sample",
	})
	if !strings.Contains(res.Output, "Synced skill 'workflow/sample'.") || !strings.Contains(res.Output, "Status: synced") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestSkillUninstallCmd(t *testing.T) {
	bank := &fakeGovernedSkillBank{
		uninstallResult: &skillbank.UninstallResult{
			SkillID:      "workflow/sample",
			Status:       "uninstalled",
			RemovedPath:  "/tmp/.openscholar/skills/_managed/workflow/sample",
			ManifestPath: "/tmp/.openscholar/skills/_managed/manifest.json",
		},
	}
	res := (&skillUninstallCmd{}).Execute(command.Context{
		App:  &app.App{SkillBank: bank},
		Args: "workflow/sample",
	})
	if !strings.Contains(res.Output, "Uninstalled skill 'workflow/sample'.") || !strings.Contains(res.Output, "Status: uninstalled") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestSkillInfoCmd(t *testing.T) {
	bank := &fakeGovernedSkillBank{
		infoResult: &skillbank.GovernedSkill{
			Meta: skillbank.SkillMeta{
				ID:          "workflow/sample",
				Name:        "sample",
				Description: "sample desc",
				Source:      "bundle",
			},
			ActiveSource: &skillbank.SkillSource{
				SourceTier: "user-managed",
				SourceKind: "local-file",
				SourceKey:  "/tmp/workflow/sample.md",
				FilePath:   "/tmp/.openscholar/skills/_managed/workflow/sample/SKILL.md",
				IsActive:   true,
			},
			Sources: []skillbank.SkillSource{
				{
					SourceTier: "user-managed",
					SourceKind: "local-file",
					SourceKey:  "/tmp/workflow/sample.md",
					FilePath:   "/tmp/.openscholar/skills/_managed/workflow/sample/SKILL.md",
					IsActive:   true,
				},
			},
		},
	}

	res := (&skillInfoCmd{}).Execute(command.Context{
		App:  &app.App{SkillBank: bank},
		Args: "workflow/sample",
	})
	for _, want := range []string{
		"## sample [workflow/sample]",
		"Active source: user-managed / local-file",
		"Declared source: bundle",
	} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("missing %q in output:\n%s", want, res.Output)
		}
	}
}
