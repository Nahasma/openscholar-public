package agent

import (
	"context"
	"os"
	"strings"
	"sync"
)

// FileDelta describes changes to a single file in a turn.
type FileDelta struct {
	FilePath     string
	AddedLines   int
	DeletedLines int
	ChangeType   string // "modified" | "created" | "deleted"
}

// TurnDiff summarizes all file changes in a single turn.
type TurnDiff struct {
	SessionID string
	TurnID    string
	Files     []FileDelta
	TotalAdd  int
	TotalDel  int
}

// TurnDiffStore builds diffs from checkpoint data.
type TurnDiffStore interface {
	BuildFromTurn(ctx context.Context, sessionID, turnID string) (TurnDiff, error)
	Latest(ctx context.Context, sessionID string) (TurnDiff, bool)
}

type turnDiffStore struct {
	checkpoints CheckpointStore
	mu          sync.Mutex
	latest      map[string]TurnDiff // sessionID → latest diff
}

// NewTurnDiffStore creates a TurnDiffStore backed by a CheckpointStore.
func NewTurnDiffStore(checkpoints CheckpointStore) TurnDiffStore {
	return &turnDiffStore{
		checkpoints: checkpoints,
		latest:      make(map[string]TurnDiff),
	}
}

// BuildFromTurn computes the diff for all files changed in a given turn.
func (s *turnDiffStore) BuildFromTurn(ctx context.Context, sessionID, turnID string) (TurnDiff, error) {
	checkpoints, err := s.checkpoints.ListByTurn(ctx, sessionID, turnID)
	if err != nil {
		return TurnDiff{}, err
	}

	diff := TurnDiff{
		SessionID: sessionID,
		TurnID:    turnID,
	}

	for _, cp := range checkpoints {
		delta := FileDelta{
			FilePath: cp.FilePath,
		}

		if !cp.ExistsBefore {
			// File was created in this turn
			delta.ChangeType = "created"
			currentContent, err := os.ReadFile(cp.FilePath)
			if err == nil {
				delta.AddedLines = countLines(string(currentContent))
			}
		} else {
			// File was modified — compare before vs current
			delta.ChangeType = "modified"
			var beforeContent string
			if cp.SnapshotPath != "" {
				if data, err := os.ReadFile(cp.SnapshotPath); err == nil {
					beforeContent = string(data)
				}
			}
			currentContent, err := os.ReadFile(cp.FilePath)
			if err != nil {
				// File was deleted after checkpoint
				delta.ChangeType = "deleted"
				delta.DeletedLines = countLines(beforeContent)
			} else {
				added, deleted := diffLineCount(beforeContent, string(currentContent))
				delta.AddedLines = added
				delta.DeletedLines = deleted
			}
		}

		diff.Files = append(diff.Files, delta)
		diff.TotalAdd += delta.AddedLines
		diff.TotalDel += delta.DeletedLines
	}

	s.mu.Lock()
	s.latest[sessionID] = diff
	s.mu.Unlock()

	return diff, nil
}

// Latest returns the most recently computed diff for a session.
func (s *turnDiffStore) Latest(ctx context.Context, sessionID string) (TurnDiff, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	diff, ok := s.latest[sessionID]
	return diff, ok
}

// countLines returns the number of lines in s.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// diffLineCount computes a simple line-based diff count.
func diffLineCount(before, after string) (added, deleted int) {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	// Build set of before lines
	beforeSet := make(map[string]int)
	for _, line := range beforeLines {
		beforeSet[line]++
	}

	// Count lines in after that aren't in before
	afterSet := make(map[string]int)
	for _, line := range afterLines {
		afterSet[line]++
	}

	for line, count := range afterSet {
		bc := beforeSet[line]
		if count > bc {
			added += count - bc
		}
	}
	for line, count := range beforeSet {
		ac := afterSet[line]
		if count > ac {
			deleted += count - ac
		}
	}

	return added, deleted
}
