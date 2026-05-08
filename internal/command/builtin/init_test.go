package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/template"
)

func TestInitConfigRoutesToConfigWizardAction(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")
	if _, err := config.Load(t.TempDir()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "config"})
	if res.Action != "config-wizard" {
		t.Fatalf("expected action config-wizard, got %q", res.Action)
	}
	if res.Output != "" {
		t.Fatalf("expected empty output for config alias, got %q", res.Output)
	}
}

func TestInitConfigRequiresLoadedConfig(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "config"})
	if res.Action != "" {
		t.Fatalf("expected no action without loaded config, got %q", res.Action)
	}
	if !strings.Contains(res.Output, "配置未加载") {
		t.Fatalf("expected config-not-loaded output, got %q", res.Output)
	}
}

func TestInitUsageMentionsConfigAliasWithoutExitHint(t *testing.T) {
	usage := initUsage()
	if !strings.Contains(usage, "等同 /config") {
		t.Fatalf("expected usage to mention /config alias, got: %s", usage)
	}
	if strings.Contains(usage, "需退出 TUI") {
		t.Fatalf("usage should not mention exiting TUI: %s", usage)
	}
}

func TestInitProfileRoutesToInitWizardAction(t *testing.T) {
	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "profile"})
	if res.Action != "init-wizard" {
		t.Fatalf("expected action init-wizard, got %q", res.Action)
	}
}

func TestInitDefaultShowsCenter(t *testing.T) {
	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{})
	if res.Action != "" {
		t.Fatalf("expected no action for /init, got %q", res.Action)
	}
	if !strings.Contains(res.Output, "Init Center") {
		t.Fatalf("expected init center output, got: %s", res.Output)
	}
}

func TestInitTemplateDryRunByDefault(t *testing.T) {
	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "template missing"})
	if res.Error == nil {
		t.Fatalf("expected template error for missing template")
	}
}

func TestInitTemplateArgParsing(t *testing.T) {
	name, apply, overwrite := parseTemplateArgs("icml2026 --apply --overwrite")
	if name != "icml2026" || !apply || !overwrite {
		t.Fatalf("got name=%q apply=%v overwrite=%v", name, apply, overwrite)
	}
}

func TestInitProjectCreatesOnlyOnce(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	cmd := &initBuiltinCmd{}
	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	first := cmd.Execute(command.Context{Args: "project"})
	if first.Error != nil {
		t.Fatalf("first create failed: %v", first.Error)
	}
	path := filepath.Join(tmp, ".openscholar", "prompt.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected prompt created: %v", err)
	}
	second := cmd.Execute(command.Context{Args: "project"})
	if second.Error != nil {
		t.Fatalf("second run failed: %v", second.Error)
	}
	if !strings.Contains(second.Output, "已存在") {
		t.Fatalf("expected no-overwrite message, got: %s", second.Output)
	}
}

func TestInitProjectUsesActiveWorkspace(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")

	workspace := t.TempDir()
	if _, err := config.Load(workspace); err != nil {
		t.Fatalf("load config: %v", err)
	}

	wd, _ := os.Getwd()
	other := t.TempDir()
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "project"})
	if res.Error != nil {
		t.Fatalf("project init failed: %v", res.Error)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".openscholar", "prompt.md")); err != nil {
		t.Fatalf("expected prompt in active workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, ".openscholar", "prompt.md")); !os.IsNotExist(err) {
		t.Fatalf("unexpected prompt in process cwd")
	}
}

func TestInitTemplateApplyRequiresOverwriteForExistingFiles(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	oldFS := template.TemplatesFS
	t.Cleanup(func() { template.TemplatesFS = oldFS })
	template.TemplatesFS = fstest.MapFS{
		"demo/template.yaml": &fstest.MapFile{Data: []byte("name: Demo\n")},
		"demo/a.txt":         &fstest.MapFile{Data: []byte("new")},
	}

	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &initBuiltinCmd{}
	res := cmd.Execute(command.Context{Args: "template demo --apply"})
	if res.Error != nil {
		t.Fatalf("expected guarded output, got error: %v", res.Error)
	}
	if !strings.Contains(res.Output, "--overwrite") {
		t.Fatalf("expected overwrite guidance, got: %s", res.Output)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("file was overwritten without --overwrite: %q", string(data))
	}

	res = cmd.Execute(command.Context{Args: "template demo --apply --overwrite"})
	if res.Error != nil {
		t.Fatalf("apply with overwrite failed: %v", res.Error)
	}
	data, err = os.ReadFile(filepath.Join(tmp, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("expected overwritten file, got %q", string(data))
	}
}
