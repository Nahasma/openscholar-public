package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	initwizard "github.com/Nahasma/openscholar-public/internal/init"
	"github.com/Nahasma/openscholar-public/internal/template"
)

type initBuiltinCmd struct{}

func (c *initBuiltinCmd) Name() string        { return "init" }
func (c *initBuiltinCmd) Description() string { return "初始化项目 (soft/config/template)" }
func (c *initBuiltinCmd) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:        "init",
		Category:    "setup",
		Source:      "builtin",
		Description: c.Description(),
		Usage:       "/init [soft|profile|config|status|project|template <name> [--apply] [--overwrite]]",
		Subcommands: []command.CommandSpec{
			{Name: "soft", Description: "打开初始化向导"},
			{Name: "profile", Description: "等同 /init soft"},
			{Name: "config", Description: "打开配置向导"},
			{Name: "status", Description: "查看初始化状态"},
			{Name: "project", Description: "初始化 .openscholar/prompt.md（如缺失）"},
			{Name: "template", ArgumentHint: "<name> [--apply] [--overwrite]", Description: "模板初始化计划（默认 dry-run）"},
		},
	}
}

func (c *initBuiltinCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)

	// Normalize --flag style to subcommand style
	args = strings.TrimPrefix(args, "--")

	switch {
	case args == "":
		return c.showCenter()

	case args == "soft" || args == "profile":
		// Open TUI init wizard dialog
		return command.Result{
			Action:  "init-wizard",
			Actions: []command.CommandAction{{Kind: command.CommandActionInitWizard}},
		}

	case args == "help":
		return command.Result{Output: initUsage(), OutputKind: command.OutputTranscriptBlock}

	case args == "config":
		if config.Get() == nil {
			return command.Result{Output: "配置未加载。请先运行 openscholar init 或设置 provider API key 后重试。"}
		}
		return command.Result{
			Action:  "config-wizard",
			Actions: []command.CommandAction{{Kind: command.CommandActionConfigWizard}},
		}

	case args == "status":
		return c.showStatus()

	case args == "project":
		return c.ensureProjectPrompt()

	case strings.HasPrefix(args, "template "):
		templateName, apply, overwrite := parseTemplateArgs(strings.TrimSpace(strings.TrimPrefix(args, "template ")))
		if templateName == "" {
			return command.Result{Output: initTemplateList()}
		}
		workspace := activeWorkspace()
		plan, err := template.PlanInit(templateName, workspace)
		if err != nil {
			return command.Result{Error: fmt.Errorf("模板初始化失败: %w", err)}
		}
		if !apply {
			return command.Result{Output: renderTemplatePlan(plan), OutputKind: command.OutputTranscriptBlock}
		}
		if plan.HasOverwriteRisk() && !overwrite {
			return command.Result{Output: renderTemplatePlan(plan) + "\n检测到已存在文件，未写入。若确认覆盖，请使用 --apply --overwrite。\n", OutputKind: command.OutputTranscriptBlock}
		}
		if err := template.ApplyInitPlanWithOptions(plan, template.ApplyOptions{Overwrite: overwrite}); err != nil {
			return command.Result{Error: fmt.Errorf("模板初始化失败: %w", err)}
		}
		return command.Result{
			Output: fmt.Sprintf("已应用模板 '%s'", templateName),
			Action: "cd", // trigger file index refresh
			Actions: []command.CommandAction{
				{Kind: command.CommandActionCD},
			},
		}

	default:
		return command.Result{Output: initUsage(), OutputKind: command.OutputTranscriptBlock}
	}
}

func initUsage() string {
	var sb strings.Builder
	sb.WriteString("Init 命令用法:\n\n")
	sb.WriteString("  /init                   Init Center 概览\n")
	sb.WriteString("  /init soft              打开个性化配置向导\n")
	sb.WriteString("  /init profile           等同 /init soft\n")
	sb.WriteString("  /init config            打开配置向导（兼容别名，等同 /config）\n")
	sb.WriteString("  /init status            查看初始化状态\n")
	sb.WriteString("  /init project           初始化项目 prompt（仅在缺失时创建）\n")
	sb.WriteString("  /init template <name>   查看模板写入计划（dry-run）\n")
	sb.WriteString("  /init template <name> --apply   应用模板到当前工作区\n")
	sb.WriteString("  /init template <name> --apply --overwrite   覆盖已有文件并应用模板\n")
	sb.WriteString("\n")
	sb.WriteString(initTemplateList())
	return sb.String()
}

func initTemplateList() string {
	templates := template.List()
	if len(templates) == 0 {
		return "暂无可用模板"
	}
	var sb strings.Builder
	sb.WriteString("可用模板:\n")
	for _, t := range templates {
		sb.WriteString(fmt.Sprintf("  - %s\n", t))
	}
	return sb.String()
}

func (c *initBuiltinCmd) showCenter() command.Result {
	status := c.statusLines()
	var sb strings.Builder
	sb.WriteString("Init Center\n\n")
	for _, line := range status {
		sb.WriteString("  " + line + "\n")
	}
	sb.WriteString("\n下一步:\n")
	sb.WriteString("  /init soft                打开 profile 向导\n")
	sb.WriteString("  /init status              查看完整初始化状态\n")
	sb.WriteString("  /init project             生成 .openscholar/prompt.md（若缺失）\n")
	sb.WriteString("  /init template <name>     预览模板写入计划\n")
	sb.WriteString("  /init template <name> --apply   实际应用模板（不覆盖已有文件）\n")
	sb.WriteString("  /init template <name> --apply --overwrite   覆盖已有文件并应用模板\n")
	sb.WriteString("  /init config              打开配置向导\n")
	return command.Result{Output: sb.String(), OutputKind: command.OutputCommandBlock}
}

func (c *initBuiltinCmd) showStatus() command.Result {
	status := c.statusLines()
	var sb strings.Builder
	sb.WriteString("Init Status\n\n")
	for _, line := range status {
		sb.WriteString("  " + line + "\n")
	}
	return command.Result{Output: sb.String(), OutputKind: command.OutputCommandBlock}
}

func (c *initBuiltinCmd) statusLines() []string {
	cwd := activeWorkspace()
	promptPath := initwizard.ProjectPromptPath(cwd)
	profilePath := initwizard.ProfilePath(cwd)
	configPath := config.ConfigFilePath(cwd)

	out := []string{
		fmt.Sprintf(".openscholar/prompt.md: %s", existsLabel(promptPath)),
		fmt.Sprintf(".openscholar/profile.yaml: %s", existsLabel(profilePath)),
		fmt.Sprintf(".openscholar/config.json: %s", existsLabel(configPath)),
	}

	detected := template.Detect(cwd)
	if detected == nil {
		out = append(out, "detected template: none")
	} else {
		out = append(out, fmt.Sprintf("detected template: %s", detected.Name))
	}
	return out
}

func (c *initBuiltinCmd) ensureProjectPrompt() command.Result {
	cwd := activeWorkspace()
	created, path, preview, err := initwizard.EnsureProjectPrompt(cwd)
	if err != nil {
		return command.Result{Error: err}
	}
	relPath := path
	if rp, relErr := filepath.Rel(cwd, path); relErr == nil {
		relPath = rp
	}
	if created {
		return command.Result{Output: fmt.Sprintf("已创建 %s\n\n%s", relPath, preview), OutputKind: command.OutputCommandBlock}
	}
	return command.Result{Output: fmt.Sprintf("%s 已存在，未覆盖\n\n%s", relPath, preview), OutputKind: command.OutputCommandBlock}
}

func parseTemplateArgs(raw string) (name string, apply bool, overwrite bool) {
	parts := strings.Fields(raw)
	for _, p := range parts {
		switch p {
		case "--apply":
			apply = true
			continue
		case "--overwrite":
			overwrite = true
			continue
		}
		if name == "" {
			name = p
		}
	}
	return name, apply, overwrite
}

func renderTemplatePlan(plan *template.InitPlan) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Template Plan: %s\n\n", plan.TemplateName))
	sb.WriteString("  mode: dry-run (no files written)\n")
	if plan.HasOverwriteRisk() {
		sb.WriteString("  use --apply to write new files only\n")
		sb.WriteString("  use --apply --overwrite to replace existing files\n\n")
	} else {
		sb.WriteString("  use --apply to write files\n\n")
	}
	for _, f := range plan.Files {
		op := "create"
		if f.Exists {
			op = "overwrite-risk"
		}
		sb.WriteString(fmt.Sprintf("  - [%s] %s\n", op, f.RelativePath))
	}
	return sb.String()
}

func activeWorkspace() string {
	return config.WorkingDirectory()
}

func existsLabel(path string) string {
	_, err := os.Stat(path)
	if err == nil {
		return "present"
	}
	return "missing"
}
