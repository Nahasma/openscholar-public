package custom

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

// SkillActivator listens to file-tool activity and lazily activates
// user-invocable skills whose `paths` frontmatter matches the touched files.
type SkillActivator struct {
	bank      skillbank.Service
	registry  *command.Registry
	mu        sync.Mutex
	activated map[string]bool
}

func NewSkillActivator(bank skillbank.Service, registry *command.Registry) *SkillActivator {
	return &SkillActivator{
		bank:      bank,
		registry:  registry,
		activated: make(map[string]bool),
	}
}

func (a *SkillActivator) OnFileToolUsage(sessionID string, evt tools.FileToolUsageEvent) {
	if a == nil || len(evt.Paths) == 0 {
		return
	}

	ctx := context.Background()
	metas, err := a.bank.ListMetadata(ctx)
	if err != nil {
		log.Printf("[skill-activator] list metadata failed: %v", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, meta := range metas {
		if a.activated[meta.ID] || !meta.IsUserInvocable() || len(meta.Paths) == 0 {
			continue
		}
		if !matchesAnyTouchedPath(evt.Workspace, evt.Paths, meta.Paths) {
			continue
		}
		if !registerSkillBankCommand(a.registry, a.bank, meta) {
			// Collision is terminal for this registry lifetime; avoid repeated attempts.
			if a.registry.Get(skillCommandName(meta)) != nil {
				a.activated[meta.ID] = true
				continue
			}
			log.Printf("[skill-activator] register %s failed: unresolved command name", meta.ID)
			continue
		}
		a.activated[meta.ID] = true
	}
}

func matchesAnyTouchedPath(workspace string, paths []string, patterns []string) bool {
	for _, touched := range paths {
		if matchesSkillPath(workspace, touched, patterns) {
			return true
		}
	}
	return false
}

func matchesSkillPath(workspace string, touched string, patterns []string) bool {
	if touched == "" {
		return false
	}

	candidates := []string{
		filepath.ToSlash(filepath.Clean(touched)),
		filepath.ToSlash(filepath.Base(touched)),
	}

	if workspace != "" {
		if rel, err := filepath.Rel(workspace, touched); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			candidates = append(candidates, filepath.ToSlash(filepath.Clean(rel)))
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if matched, err := doublestar.Match(pattern, candidate); err == nil && matched {
				return true
			}
		}
	}
	return false
}
