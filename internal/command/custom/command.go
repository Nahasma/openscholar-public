package custom

import "github.com/Nahasma/openscholar-public/internal/command"

// CustomCommand implements command.Command for user-defined markdown commands.
type CustomCommand struct {
	name        string
	description string
	body        string
	frontmatter Frontmatter
}

func (c *CustomCommand) Name() string        { return c.name }
func (c *CustomCommand) Description() string { return c.description }
func (c *CustomCommand) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:         c.name,
		Category:     "custom",
		Source:       "custom",
		Description:  c.description,
		ArgumentHint: c.frontmatter.ArgumentHint,
		WhenToUse:    c.frontmatter.WhenToUse,
	}
}

// AllowedTools returns the tools this command is allowed to use (empty = no restriction).
func (c *CustomCommand) AllowedTools() []string { return c.frontmatter.AllowedTools }

// Model returns the model alias/ID override for this command (empty = use default).
func (c *CustomCommand) Model() string { return c.frontmatter.Model }

// WhenToUse returns the hint for automatic command selection.
func (c *CustomCommand) WhenToUse() string { return c.frontmatter.WhenToUse }

// ArgumentHint returns the human-readable argument hint for help display.
func (c *CustomCommand) ArgumentHint() string { return c.frontmatter.ArgumentHint }

func (c *CustomCommand) Execute(ctx command.Context) command.Result {
	expanded := ExpandBodyFM(c.body, ctx.Args, c.frontmatter)
	return command.Result{
		Prompt: expanded,
		Runtime: &command.RuntimeOverride{
			AllowedTools:           append([]string(nil), c.frontmatter.AllowedTools...),
			Model:                  c.frontmatter.Model,
			DisableModelInvocation: c.frontmatter.DisableModelInvocation || c.frontmatter.DisableModelInvocationAlt,
		},
	}
}
