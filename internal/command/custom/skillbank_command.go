package custom

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"unicode"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/skillbank"
)

// LoadSkillBankCommands registers user-invocable SkillBank skills as slash commands.
// Built-ins/custom commands always win on name collisions; colliding skills are skipped.
func LoadSkillBankCommands(registry *command.Registry, skills skillbank.Service) {
	if registry == nil || skills == nil {
		return
	}

	metas, err := skills.ListMetadata(context.Background())
	if err != nil {
		log.Printf("[command] failed to load skillbank metadata: %v", err)
		return
	}

	for _, meta := range metas {
		if !meta.IsUserInvocable() || len(meta.Paths) > 0 {
			continue
		}
		registerSkillBankCommand(registry, skills, meta)
	}
}

// SkillBankCommand executes a skill instruction as a slash command prompt.
type SkillBankCommand struct {
	name           string
	description    string
	whenToUse      string
	allowed        []string
	model          string
	exposure       skillbank.SkillExposure
	userInvocable  bool
	modelInvocable bool
	skillID        string
	skills         skillbank.Service
}

func (c *SkillBankCommand) Name() string        { return c.name }
func (c *SkillBankCommand) Description() string { return c.description }
func (c *SkillBankCommand) WhenToUse() string   { return c.whenToUse }
func (c *SkillBankCommand) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:           c.name,
		Category:       "skill",
		Source:         "skill",
		Description:    c.description,
		WhenToUse:      c.whenToUse,
		Exposure:       string(c.exposure),
		UserInvocable:  c.userInvocable,
		ModelInvocable: c.modelInvocable,
		Hidden:         !c.userInvocable,
	}
}
func (c *SkillBankCommand) AllowedTools() []string {
	return c.allowed
}
func (c *SkillBankCommand) Model() string { return c.model }

func (c *SkillBankCommand) Execute(ctx command.Context) command.Result {
	runCtx := ctx.ExecContext
	if runCtx == nil {
		runCtx = context.Background()
	}
	skill, err := c.skills.View(runCtx, c.skillID)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Failed to load skill %s: %v", c.skillID, err)}
	}
	return command.Result{
		Prompt: ExpandBody(skill.Instruction, ctx.Args),
		Runtime: &command.RuntimeOverride{
			AllowedTools: append([]string(nil), c.allowed...),
			Model:        c.model,
		},
	}
}

func registerSkillBankCommand(registry *command.Registry, skills skillbank.Service, meta skillbank.SkillMeta) bool {
	name := skillCommandName(meta)
	if name == "" {
		return false
	}
	if existing := registry.Get(name); existing != nil {
		log.Printf("[command] skip skill command /%s (%s): name already registered", name, meta.ID)
		return false
	}

	desc := strings.TrimSpace(meta.Description)
	if desc == "" {
		desc = "Skill command: " + meta.ID
	}

	registry.Register(&SkillBankCommand{
		name:           name,
		description:    desc,
		whenToUse:      strings.TrimSpace(meta.WhenToUse),
		allowed:        append([]string(nil), meta.AllowedTools...),
		model:          meta.Model,
		exposure:       meta.Exposure,
		userInvocable:  meta.IsUserInvocable(),
		modelInvocable: meta.IsModelInvocable(),
		skillID:        meta.ID,
		skills:         skills,
	})
	return true
}

func skillCommandName(meta skillbank.SkillMeta) string {
	candidate := strings.TrimSpace(path.Base(meta.ID))
	if candidate == "." || candidate == "/" {
		candidate = ""
	}
	if candidate == "" {
		candidate = strings.TrimSpace(meta.Name)
	}
	return sanitizeCommandName(candidate)
}

func sanitizeCommandName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	prevUnderscore := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if r == '-' || r == '_' {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_-")
	if out == "" {
		return ""
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "skill_" + out
	}
	return out
}
