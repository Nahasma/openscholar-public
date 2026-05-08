package command

import (
	"sort"
	"strings"
	"sync"
)

// Registry holds all registered commands.
type Registry struct {
	mu       sync.RWMutex
	commands map[string]Command
	aliases  map[string]string
	specs    map[string]CommandSpec
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
		specs:    make(map[string]CommandSpec),
	}
}

// Register adds a command to the registry.
func (r *Registry) Register(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := cmd.Name()
	r.commands[name] = cmd
	delete(r.aliases, name)
	for alias, canonical := range r.aliases {
		if canonical == name {
			delete(r.aliases, alias)
		}
	}
	r.specs[name] = commandSpecFor(cmd)
	for _, alias := range r.specs[name].Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == name {
			continue
		}
		if _, exists := r.commands[alias]; exists {
			continue
		}
		if _, exists := r.aliases[alias]; exists {
			continue
		}
		r.aliases[alias] = name
	}
}

// Get returns a command by name, or nil if not found.
func (r *Registry) Get(name string) Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cmd, ok := r.commands[name]; ok {
		return cmd
	}
	if canonical, ok := r.aliases[name]; ok {
		return r.commands[canonical]
	}
	return nil
}

// List returns all registered command names sorted alphabetically.
func (r *Registry) List() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name() < cmds[j].Name()
	})
	return cmds
}

// Complete returns command names that start with the given prefix.
func (r *Registry) Complete(prefix string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matches []string
	added := make(map[string]struct{}, len(r.commands)+len(r.aliases))
	for name := range r.commands {
		if strings.HasPrefix(name, prefix) {
			added[name] = struct{}{}
			matches = append(matches, name)
		}
	}
	for alias := range r.aliases {
		if strings.HasPrefix(alias, prefix) {
			if _, ok := added[alias]; ok {
				continue
			}
			matches = append(matches, alias)
		}
	}
	sort.Strings(matches)
	return matches
}

// CompleteCommands returns Command objects whose names start with the given prefix.
func (r *Registry) CompleteCommands(prefix string) []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matches []Command
	added := make(map[string]struct{}, len(r.commands))
	for name, cmd := range r.commands {
		if strings.HasPrefix(name, prefix) {
			added[name] = struct{}{}
			matches = append(matches, cmd)
		}
	}
	for alias, canonical := range r.aliases {
		if !strings.HasPrefix(alias, prefix) {
			continue
		}
		if _, ok := added[canonical]; ok {
			continue
		}
		if cmd, ok := r.commands[canonical]; ok {
			added[canonical] = struct{}{}
			matches = append(matches, cmd)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name() < matches[j].Name()
	})
	return matches
}

// CompleteInvocation returns command/subcommand completion candidates at cursor.
func (r *Registry) CompleteInvocation(raw string, cursor int) (CompletionContext, []CompletionItem) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ctx := CompletionContext{Invocation: ParseInvocation(raw, cursor)}
	if !ctx.Invocation.IsSlash {
		return ctx, nil
	}

	if ctx.Invocation.Command == "" || ctx.Invocation.CursorInCommand {
		ctx.Target = "command"
		prefix := ctx.Invocation.CommandPrefix
		if prefix == "" {
			prefix = ctx.Invocation.Command
		}
		ctx.Prefix = prefix
		return ctx, r.completeCommandsLocked(prefix, false)
	}

	spec, ok := r.specForNameLocked(ctx.Invocation.Command)
	if !ok || len(spec.Subcommands) == 0 {
		return ctx, nil
	}
	if !ctx.Invocation.CursorInSubcommand {
		return ctx, nil
	}
	ctx.Target = "subcommand"
	ctx.Prefix = ctx.Invocation.SubcommandPrefix
	items := make([]CompletionItem, 0, len(spec.Subcommands))
	for _, sub := range spec.Subcommands {
		if !strings.HasPrefix(sub.Name, ctx.Prefix) {
			continue
		}
		items = append(items, CompletionItem{
			Kind:         "subcommand",
			Value:        sub.Name,
			CommandName:  ctx.Invocation.Command,
			Subcommand:   sub.Name,
			Description:  sub.Description,
			ArgumentHint: sub.ArgumentHint,
			WhenToUse:    sub.WhenToUse,
			Source:       spec.Source,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value < items[j].Value })
	return ctx, items
}

func (r *Registry) completeCommandsLocked(prefix string, includeHidden bool) []CompletionItem {
	items := make([]CompletionItem, 0, len(r.specs)+len(r.aliases))
	added := make(map[string]struct{}, len(r.specs)+len(r.aliases))
	for name, spec := range r.specs {
		if !includeHidden && !spec.UserInvocable {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if _, ok := added[name]; ok {
			continue
		}
		added[name] = struct{}{}
		items = append(items, CompletionItem{
			Kind:         "command",
			Value:        name,
			CommandName:  name,
			Description:  spec.Description,
			ArgumentHint: spec.ArgumentHint,
			WhenToUse:    spec.WhenToUse,
			Source:       spec.Source,
		})
	}
	for alias, canonical := range r.aliases {
		if !strings.HasPrefix(alias, prefix) {
			continue
		}
		spec, ok := r.specs[canonical]
		if !ok {
			continue
		}
		if !includeHidden && !spec.UserInvocable {
			continue
		}
		if _, ok := added[alias]; ok {
			continue
		}
		added[alias] = struct{}{}
		items = append(items, CompletionItem{
			Kind:         "command",
			Value:        alias,
			CommandName:  canonical,
			Description:  spec.Description,
			ArgumentHint: spec.ArgumentHint,
			WhenToUse:    spec.WhenToUse,
			Source:       spec.Source,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value < items[j].Value })
	return items
}

func (r *Registry) specForNameLocked(name string) (CommandSpec, bool) {
	if spec, ok := r.specs[name]; ok {
		return spec, true
	}
	if canonical, ok := r.aliases[name]; ok {
		spec, ok := r.specs[canonical]
		return spec, ok
	}
	return CommandSpec{}, false
}

// Spec returns the command spec by name or alias.
func (r *Registry) Spec(name string) (CommandSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if spec, ok := r.specs[name]; ok {
		return spec, true
	}
	if canonical, ok := r.aliases[name]; ok {
		spec, ok := r.specs[canonical]
		return spec, ok
	}
	return CommandSpec{}, false
}

// ListSpecs returns user-invocable command specs sorted by command name.
func (r *Registry) ListSpecs() []CommandSpec {
	return r.ListUserInvocableSpecs()
}

func (r *Registry) ListUserInvocableSpecs() []CommandSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]CommandSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		if !spec.UserInvocable {
			continue
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func (r *Registry) ListModelInvocableSpecs() []CommandSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]CommandSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		if !spec.ModelInvocable {
			continue
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func (r *Registry) ListAllSpecs() []CommandSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]CommandSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func commandSpecFor(cmd Command) CommandSpec {
	if provider, ok := cmd.(SpecProvider); ok {
		spec := provider.Spec()
		if strings.TrimSpace(spec.Name) == "" {
			spec.Name = cmd.Name()
		}
		if strings.TrimSpace(spec.Description) == "" {
			spec.Description = cmd.Description()
		}
		if strings.TrimSpace(spec.Exposure) == "" {
			spec.Exposure = "explicit"
		}
		if !spec.UserInvocable && !spec.ModelInvocable && spec.Exposure == "explicit" {
			spec.UserInvocable = true
			spec.ModelInvocable = false
		}
		spec.Hidden = !spec.UserInvocable
		return spec
	}
	return CommandSpec{
		Name:           cmd.Name(),
		Description:    cmd.Description(),
		Exposure:       "explicit",
		UserInvocable:  true,
		ModelInvocable: false,
		Hidden:         false,
	}
}
