package builtin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
)

type helpCmd struct {
	registry *command.Registry
}

func (c *helpCmd) Name() string        { return "help" }
func (c *helpCmd) Description() string { return "Show available commands and shortcuts" }
func (c *helpCmd) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:           "help",
		Category:       "help",
		Source:         "builtin",
		Description:    c.Description(),
		ArgumentHint:   "[command|--all|--hidden|--implicit|--skills]",
		Usage:          "/help [command|--all|--hidden|--implicit|--skills]",
		WhenToUse:      "List slash commands or inspect one command's subcommands, aliases, and metadata.",
		Exposure:       "explicit",
		UserInvocable:  true,
		ModelInvocable: false,
	}
}

func (c *helpCmd) Execute(ctx command.Context) command.Result {
	if c.registry == nil {
		return command.Result{Output: "No command registry is available.", OutputKind: command.OutputTranscriptBlock}
	}
	args := strings.TrimSpace(ctx.Args)
	if args != "" {
		tokens := strings.Fields(args)
		for _, tok := range tokens {
			if strings.HasPrefix(tok, "-") {
				return command.Result{Output: c.renderOverviewWithFlags(tokens), OutputKind: command.OutputTranscriptBlock}
			}
		}
		return command.Result{Output: c.renderCommandHelp(strings.TrimPrefix(args, "/")), OutputKind: command.OutputTranscriptBlock}
	}
	return command.Result{Output: c.renderOverviewFromSpecs(c.registry.ListUserInvocableSpecs()), OutputKind: command.OutputTranscriptBlock}
}

func (c *helpCmd) renderOverviewWithFlags(tokens []string) string {
	showAll := false
	showHidden := false
	onlySkills := false
	for _, tok := range tokens {
		switch tok {
		case "--all":
			showAll = true
		case "--hidden", "--implicit":
			showHidden = true
		case "--skills":
			onlySkills = true
		}
	}

	specs := c.registry.ListUserInvocableSpecs()
	if showAll || showHidden {
		specs = c.registry.ListAllSpecs()
	}
	if showHidden && !showAll {
		filtered := make([]command.CommandSpec, 0, len(specs))
		for _, spec := range specs {
			if !spec.UserInvocable {
				filtered = append(filtered, spec)
			}
		}
		specs = filtered
	}
	if onlySkills {
		filtered := make([]command.CommandSpec, 0, len(specs))
		for _, spec := range specs {
			if spec.Source == "skill" {
				filtered = append(filtered, spec)
			}
		}
		specs = filtered
	}
	return c.renderOverviewFromSpecs(specs)
}

func (c *helpCmd) renderOverviewFromSpecs(specs []command.CommandSpec) string {
	var sb strings.Builder
	sb.WriteString("Slash Commands\n\n")
	if c.registry == nil {
		sb.WriteString("No command registry is available.\n")
		return sb.String()
	}

	byCategory := make(map[string][]command.CommandSpec)
	for _, spec := range specs {
		category := strings.TrimSpace(spec.Category)
		if category == "" {
			category = "general"
		}
		byCategory[category] = append(byCategory[category], spec)
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	sb.WriteString("Use /help <command> for subcommands, aliases, argument hints, and source details.\n\n")
	for _, category := range categories {
		sb.WriteString(fmt.Sprintf("%s:\n", category))
		group := byCategory[category]
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })
		for _, spec := range group {
			name := "/" + spec.Name
			hint := strings.TrimSpace(spec.ArgumentHint)
			if hint != "" {
				name += " " + hint
			}
			source := strings.TrimSpace(spec.Source)
			if source == "" {
				source = "builtin"
			}
			status := fmt.Sprintf("exp=%s user=%t model=%t", spec.Exposure, spec.UserInvocable, spec.ModelInvocable)
			sb.WriteString(fmt.Sprintf("  %-28s %-9s %s (%s)\n", name, "["+source+"]", spec.Description, status))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Custom Commands And SkillBank\n")
	sb.WriteString("  Custom command frontmatter is surfaced in picker and help: description, allowed_tools/allowed-tools, model, when_to_use, argument_hint, arguments, user_invocable, disable_model_invocation.\n")
	sb.WriteString("  SkillBank skills use exposure=implicit|explicit|both|none. Default help shows user-invocable commands; /help --all or /help --hidden shows filtered entries and model visibility.\n\n")

	sb.WriteString("Keyboard Shortcuts\n")
	sb.WriteString("  Shift+Tab    Cycle mode (default → auto → plan)\n")
	sb.WriteString("  Ctrl+O       Toggle tool call details\n")
	sb.WriteString("  Ctrl+P       Browse sessions\n")
	sb.WriteString("  Ctrl+N       New session\n")
	sb.WriteString("  Ctrl+L       Clear screen\n")
	sb.WriteString("  Ctrl+C       Cancel / double-press to quit\n")
	sb.WriteString("  Esc          Cancel current request\n")
	sb.WriteString("  ↑/↓          Browse command history\n")
	sb.WriteString("  Tab          Auto-complete commands\n")
	return sb.String()
}

func (c *helpCmd) renderCommandHelp(name string) string {
	if c.registry == nil {
		return "No command registry is available."
	}
	spec, ok := c.registry.Spec(strings.TrimSpace(name))
	if !ok {
		return fmt.Sprintf("Unknown command: /%s", name)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Command: /%s\n\n", spec.Name))
	if spec.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", spec.Description))
	}
	usage := spec.Usage
	if usage == "" {
		usage = "/" + spec.Name
		if spec.ArgumentHint != "" {
			usage += " " + spec.ArgumentHint
		}
	}
	sb.WriteString(fmt.Sprintf("Usage: %s\n", usage))
	if len(spec.Aliases) > 0 {
		sb.WriteString(fmt.Sprintf("Aliases: /%s\n", strings.Join(spec.Aliases, ", /")))
	}
	category := spec.Category
	if category == "" {
		category = "general"
	}
	source := spec.Source
	if source == "" {
		source = "builtin"
	}
	sb.WriteString(fmt.Sprintf("Category: %s\n", category))
	sb.WriteString(fmt.Sprintf("Source: %s\n", source))
	sb.WriteString(fmt.Sprintf("Exposure: %s (user=%t model=%t)\n", spec.Exposure, spec.UserInvocable, spec.ModelInvocable))
	if spec.ArgumentHint != "" {
		sb.WriteString(fmt.Sprintf("Arguments: %s\n", spec.ArgumentHint))
	}
	if spec.WhenToUse != "" {
		sb.WriteString(fmt.Sprintf("When to use: %s\n", spec.WhenToUse))
	}
	if len(spec.Subcommands) > 0 {
		sb.WriteString("\nSubcommands:\n")
		for _, sub := range spec.Subcommands {
			name := "/" + spec.Name + " " + sub.Name
			if sub.ArgumentHint != "" {
				name += " " + sub.ArgumentHint
			}
			sb.WriteString(fmt.Sprintf("  %-32s %s\n", name, sub.Description))
			if sub.WhenToUse != "" {
				sb.WriteString(fmt.Sprintf("    when: %s\n", sub.WhenToUse))
			}
		}
	}
	if source == "custom" {
		sb.WriteString("\nCustom metadata comes from .openscholar/commands/*.md frontmatter.\n")
	}
	if source == "skill" {
		sb.WriteString("\nThis command is generated from a user-invocable SkillBank skill.\n")
	}
	return sb.String()
}
