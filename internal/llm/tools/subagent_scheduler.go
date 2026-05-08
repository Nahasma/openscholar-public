package tools

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
)

// SubagentScheduler coordinates subagent fanout within a parent session.
type SubagentScheduler struct {
	enabled     bool
	globalLimit int
	mu          sync.Mutex
	byID        map[string]*schedulerSessionState
}

type schedulerSessionState struct {
	global       chan struct{}
	profiles     map[string]chan struct{}
	activeWrites map[string]struct{}
}

type subagentLease struct {
	s               *SubagentScheduler
	parentSessionID string
	profileID       string
	writeSet        []string
	released        bool
}

func newSubagentScheduler(cfg *config.Config) *SubagentScheduler {
	enabled := cfg != nil && cfg.SubagentOrchestration.Enabled
	return &SubagentScheduler{enabled: enabled, globalLimit: schedulerGlobalLimit(cfg), byID: make(map[string]*schedulerSessionState)}
}

// NewSubagentScheduler creates a scheduler from the current orchestration config.
func NewSubagentScheduler(cfg *config.Config) *SubagentScheduler {
	return newSubagentScheduler(cfg)
}

func acquireSubagentLease(ctx context.Context, scheduler *SubagentScheduler, parentSessionID string, profile SubagentProfile, writeSet []string) (*subagentLease, error) {
	if scheduler == nil {
		return &subagentLease{}, nil
	}
	return scheduler.Acquire(ctx, parentSessionID, profile, writeSet)
}

func (s *SubagentScheduler) Acquire(ctx context.Context, parentSessionID string, profile SubagentProfile, writeSet []string) (*subagentLease, error) {
	if s == nil || !s.enabled {
		return &subagentLease{}, nil
	}
	parentSessionID = strings.TrimSpace(parentSessionID)
	if parentSessionID == "" {
		return nil, fmt.Errorf("parent session is required")
	}
	profID := strings.ToLower(strings.TrimSpace(profile.ID))
	if profID == "" {
		profID = strings.ToLower(strings.TrimSpace(profile.AgentType))
	}
	if profID == "" {
		profID = "general"
	}

	ss, globalLimit := s.sessionState(parentSessionID)
	if err := acquireSlot(ctx, ss.global, globalLimit); err != nil {
		return nil, fmt.Errorf("waiting for global subagent slot canceled: %w", err)
	}
	profileLimit := s.profileLimit(profile, globalLimit)
	profileCh := s.profileSemaphore(ss, profID, profileLimit)
	if err := acquireSlot(ctx, profileCh, profileLimit); err != nil {
		releaseSlot(ss.global)
		return nil, fmt.Errorf("waiting for %s subagent slot canceled: %w", profID, err)
	}

	norm := normalizeWriteSet(writeSet)
	if err := s.waitAndLockWriteSet(ctx, ss, norm); err != nil {
		releaseSlot(profileCh)
		releaseSlot(ss.global)
		return nil, err
	}
	return &subagentLease{s: s, parentSessionID: parentSessionID, profileID: profID, writeSet: norm}, nil
}

func (l *subagentLease) Release() {
	if l == nil || l.s == nil || !l.s.enabled || l.released {
		return
	}
	l.s.release(l)
	l.released = true
}

func (s *SubagentScheduler) release(l *subagentLease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.byID[l.parentSessionID]
	if !ok {
		return
	}
	for _, p := range l.writeSet {
		delete(ss.activeWrites, p)
	}
	if ch, ok := ss.profiles[l.profileID]; ok {
		releaseSlot(ch)
	}
	releaseSlot(ss.global)
}

func (s *SubagentScheduler) sessionState(parentSessionID string) (*schedulerSessionState, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.byID[parentSessionID]
	if !ok {
		globalLimit := s.globalLimit
		if globalLimit <= 0 {
			globalLimit = 4
		}
		ss = &schedulerSessionState{
			global:       make(chan struct{}, globalLimit),
			profiles:     make(map[string]chan struct{}),
			activeWrites: make(map[string]struct{}),
		}
		s.byID[parentSessionID] = ss
	}
	return ss, cap(ss.global)
}

func (s *SubagentScheduler) profileSemaphore(ss *schedulerSessionState, profileID string, limit int) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := ss.profiles[profileID]; ok {
		return ch
	}
	ch := make(chan struct{}, limit)
	ss.profiles[profileID] = ch
	return ch
}

func (s *SubagentScheduler) waitAndLockWriteSet(ctx context.Context, ss *schedulerSessionState, writeSet []string) error {
	if len(writeSet) == 0 {
		return nil
	}
	for {
		s.mu.Lock()
		conflict := false
		for _, target := range writeSet {
			for active := range ss.activeWrites {
				if overlapsWritePath(target, active) {
					conflict = true
					break
				}
			}
			if conflict {
				break
			}
		}
		if !conflict {
			for _, p := range writeSet {
				ss.activeWrites[p] = struct{}{}
			}
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for write_set lock canceled: %w", ctx.Err())
		case <-time.After(schedulerPollInterval):
		}
	}
}

func (s *SubagentScheduler) profileLimit(profile SubagentProfile, parentMax int) int {
	if profile.MaxConcurrent > 0 {
		return profile.MaxConcurrent
	}
	agentType := strings.ToLower(strings.TrimSpace(profile.AgentType))
	switch agentType {
	case readingWorkerAgentType:
		return 5
	case "explore":
		return 4
	case "verify":
		return 2
	case "plan", "leader", "coordinator":
		return 1
	default:
		return parentMax
	}
}

func schedulerGlobalLimit(cfg *config.Config) int {
	if cfg != nil && cfg.SubagentOrchestration.GlobalMaxConcurrent > 0 {
		return cfg.SubagentOrchestration.GlobalMaxConcurrent
	}
	return 4
}

func acquireSlot(ctx context.Context, ch chan struct{}, _ int) error {
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseSlot(ch chan struct{}) {
	select {
	case <-ch:
	default:
	}
}

func normalizeWriteSet(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ToLower(path.Clean(strings.ReplaceAll(p, "\\", "/")))
		if p == "." || p == "/" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func overlapsWritePath(a, b string) bool {
	if a == b {
		return true
	}
	if strings.HasPrefix(a, b+"/") {
		return true
	}
	if strings.HasPrefix(b, a+"/") {
		return true
	}
	return false
}

var schedulerPollInterval = 20 * time.Millisecond
