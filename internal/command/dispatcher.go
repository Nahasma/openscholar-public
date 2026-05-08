package command

import "strings"

// Dispatcher parses and dispatches slash commands.
type Dispatcher struct {
	registry *Registry
}

// NewDispatcher creates a new command dispatcher.
func NewDispatcher(registry *Registry) *Dispatcher {
	return &Dispatcher{registry: registry}
}

// IsCommand returns true if the input starts with '/'.
func (d *Dispatcher) IsCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// Dispatch parses the input and executes the matching command.
func (d *Dispatcher) Dispatch(ctx Context, input string) Result {
	input = strings.TrimSpace(input)
	inv := ParseInvocation(input, len(input))
	if !inv.IsSlash {
		return Result{Error: nil}
	}
	cmd := d.registry.Get(inv.Command)
	if cmd == nil {
		return Result{Output: "Unknown command: /" + inv.Command + ". Type /help for available commands."}
	}

	ctx.Args = inv.ArgsRaw
	return cmd.Execute(ctx)
}

// Registry returns the underlying registry (for tab completion).
func (d *Dispatcher) Registry() *Registry {
	return d.registry
}
