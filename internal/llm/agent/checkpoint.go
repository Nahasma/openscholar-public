package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// FileCheckpoint represents a snapshot of a file taken before a write operation.
type FileCheckpoint struct {
	ID           string      `json:"id"`
	SessionID    string      `json:"session_id"`
	TurnID       string      `json:"turn_id"`
	FilePath     string      `json:"file_path"`
	ExistsBefore bool        `json:"exists_before"`
	SnapshotPath string      `json:"snapshot_path"`
	FileMode     fs.FileMode `json:"file_mode"`
	Size         int64       `json:"size"`
	SHA256       string      `json:"sha256"`
	CreatedAt    time.Time   `json:"created_at"`
}

// CheckpointStore provides file snapshot capabilities for undo/turn-diff.
type CheckpointStore interface {
	// CaptureBeforeWrite saves a file snapshot before a write operation.
	// For the same turn + same file, only one snapshot is taken.
	CaptureBeforeWrite(ctx context.Context, sessionID, turnID, filePath string) (FileCheckpoint, error)

	ListByTurn(ctx context.Context, sessionID, turnID string) ([]FileCheckpoint, error)
	LatestTurn(ctx context.Context, sessionID string) (string, bool)
	RestoreTurn(ctx context.Context, sessionID, turnID string) error
	Prune(ctx context.Context, sessionID string, keep int) error
}

// NewCheckpointStore creates a new CheckpointStore rooted at rootDir.
// Existing manifests are loaded on startup for recovery.
func NewCheckpointStore(rootDir string) CheckpointStore {
	cs := &checkpointStore{
		rootDir:   rootDir,
		sessions:  make(map[string]*sessionState),
	}
	return cs
}

// ─── context helpers ──────────────────────────────────────────────────────────
// NOTE (Fix 7): The canonical context keys for turnID and checkpoint store are
// defined in tools.TurnIDContextKey and tools.CheckpointStoreContextKey.
// The helpers below are kept for backward compatibility but callers should
// prefer the tools package keys, which are what loop.go actually sets.

type turnIDKeyType struct{}

var turnIDCtxKey = turnIDKeyType{}

// CurrentTurnID retrieves the turn ID from the agent-local context key.
// Prefer tools.TurnIDContextKey for cross-package use.
func CurrentTurnID(ctx context.Context) string {
	v, _ := ctx.Value(turnIDCtxKey).(string)
	return v
}

// WithTurnID stores a turn ID using the agent-local context key.
func WithTurnID(ctx context.Context, turnID string) context.Context {
	return context.WithValue(ctx, turnIDCtxKey, turnID)
}

type checkpointStoreKeyType struct{}

var checkpointStoreCtxKey = checkpointStoreKeyType{}

// CheckpointStoreFromContext retrieves a CheckpointStore from the agent-local context key.
// Prefer tools.CheckpointStoreContextKey for cross-package use.
func CheckpointStoreFromContext(ctx context.Context) CheckpointStore {
	v, _ := ctx.Value(checkpointStoreCtxKey).(CheckpointStore)
	return v
}

// WithCheckpointStore stores a CheckpointStore using the agent-local context key.
func WithCheckpointStore(ctx context.Context, cs CheckpointStore) context.Context {
	return context.WithValue(ctx, checkpointStoreCtxKey, cs)
}

// ─── internal implementation ──────────────────────────────────────────────────

type sessionState struct {
	mu         sync.Mutex
	checkpoints []FileCheckpoint
	// turnOrder tracks turn insertion order for Prune
	turnOrder  []string
	// sha256Index maps sha256 → SnapshotPath for deduplication
	sha256Index map[string]string
}

type checkpointStore struct {
	mu       sync.Mutex
	rootDir  string
	sessions map[string]*sessionState
}

// getSession lazily initialises or loads session state (caller must NOT hold cs.mu).
func (cs *checkpointStore) getSession(sessionID string) (*sessionState, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if st, ok := cs.sessions[sessionID]; ok {
		return st, nil
	}

	st := &sessionState{
		sha256Index: make(map[string]string),
	}

	// Try to load existing manifest.
	manifestPath := cs.manifestPath(sessionID)
	if f, err := os.Open(manifestPath); err == nil {
		scanner := bufio.NewScanner(f)
		turnSeen := make(map[string]struct{})
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var cp FileCheckpoint
			if err2 := json.Unmarshal(line, &cp); err2 == nil {
				st.checkpoints = append(st.checkpoints, cp)
				if _, ok := turnSeen[cp.TurnID]; !ok {
					turnSeen[cp.TurnID] = struct{}{}
					st.turnOrder = append(st.turnOrder, cp.TurnID)
				}
				if cp.SHA256 != "" && cp.SnapshotPath != "" {
					st.sha256Index[cp.SHA256] = cp.SnapshotPath
				}
			}
		}
		_ = f.Close()
	}

	cs.sessions[sessionID] = st
	return st, nil
}

func (cs *checkpointStore) manifestPath(sessionID string) string {
	return filepath.Join(cs.rootDir, sessionID, "manifest.jsonl")
}

func (cs *checkpointStore) snapshotDir(sessionID, turnID string) string {
	return filepath.Join(cs.rootDir, sessionID, turnID)
}

// CaptureBeforeWrite saves a file snapshot before it is modified.
func (cs *checkpointStore) CaptureBeforeWrite(ctx context.Context, sessionID, turnID, filePath string) (FileCheckpoint, error) {
	st, err := cs.getSession(sessionID)
	if err != nil {
		return FileCheckpoint{}, err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	// Deduplicate: same turn + same file → skip.
	for _, cp := range st.checkpoints {
		if cp.SessionID == sessionID && cp.TurnID == turnID && cp.FilePath == filePath {
			return cp, nil
		}
	}

	cp := FileCheckpoint{
		ID:        generateID(),
		SessionID: sessionID,
		TurnID:    turnID,
		FilePath:  filePath,
		CreatedAt: time.Now(),
	}

	// Stat the file.
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// File doesn't exist yet — record that, no backup needed.
		cp.ExistsBefore = false
		cp.SnapshotPath = ""
	} else if err != nil {
		return FileCheckpoint{}, fmt.Errorf("checkpoint stat %s: %w", filePath, err)
	} else {
		cp.ExistsBefore = true
		cp.FileMode = info.Mode()
		cp.Size = info.Size()

		// Read content and compute SHA256.
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return FileCheckpoint{}, fmt.Errorf("checkpoint read %s: %w", filePath, readErr)
		}
		h := sha256.Sum256(data)
		cp.SHA256 = hex.EncodeToString(h[:])

		// Deduplication: reuse an existing .bak if same content.
		if existing, ok := st.sha256Index[cp.SHA256]; ok {
			cp.SnapshotPath = existing
		} else {
			// Create the .bak file.
			snapDir := cs.snapshotDir(sessionID, turnID)
			if mkErr := os.MkdirAll(snapDir, 0o755); mkErr != nil {
				return FileCheckpoint{}, fmt.Errorf("checkpoint mkdir %s: %w", snapDir, mkErr)
			}
			snapFile := filepath.Join(snapDir, cp.SHA256[:8]+".bak")
			if writeErr := fileop.WriteFileAtomic(snapFile, data, 0o600); writeErr != nil {
				return FileCheckpoint{}, fmt.Errorf("checkpoint write snap %s: %w", snapFile, writeErr)
			}
			cp.SnapshotPath = snapFile
			st.sha256Index[cp.SHA256] = snapFile
		}
	}

	// Persist to manifest.
	if err2 := cs.appendManifest(sessionID, cp); err2 != nil {
		return FileCheckpoint{}, err2
	}

	// Update in-memory state.
	st.checkpoints = append(st.checkpoints, cp)
	// Track turn order.
	alreadyTracked := false
	for _, t := range st.turnOrder {
		if t == turnID {
			alreadyTracked = true
			break
		}
	}
	if !alreadyTracked {
		st.turnOrder = append(st.turnOrder, turnID)
	}

	return cp, nil
}

// appendManifest atomically appends a single checkpoint line to the manifest file.
func (cs *checkpointStore) appendManifest(sessionID string, cp FileCheckpoint) error {
	dir := filepath.Join(cs.rootDir, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("checkpoint manifest dir: %w", err)
	}

	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("checkpoint marshal: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(cs.manifestPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("checkpoint open manifest: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// ListByTurn returns all checkpoints for the given session + turn.
func (cs *checkpointStore) ListByTurn(ctx context.Context, sessionID, turnID string) ([]FileCheckpoint, error) {
	st, err := cs.getSession(sessionID)
	if err != nil {
		return nil, err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	var out []FileCheckpoint
	for _, cp := range st.checkpoints {
		if cp.SessionID == sessionID && cp.TurnID == turnID {
			out = append(out, cp)
		}
	}
	return out, nil
}

// LatestTurn returns the most recent turn ID for the session.
func (cs *checkpointStore) LatestTurn(ctx context.Context, sessionID string) (string, bool) {
	st, err := cs.getSession(sessionID)
	if err != nil {
		return "", false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.turnOrder) == 0 {
		return "", false
	}
	return st.turnOrder[len(st.turnOrder)-1], true
}

// RestoreTurn restores all files captured in the given turn to their pre-write state.
func (cs *checkpointStore) RestoreTurn(ctx context.Context, sessionID, turnID string) error {
	st, err := cs.getSession(sessionID)
	if err != nil {
		return err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	for _, cp := range st.checkpoints {
		if cp.SessionID != sessionID || cp.TurnID != turnID {
			continue
		}

		if !cp.ExistsBefore {
			// File did not exist before the turn — delete it.
			if removeErr := os.Remove(cp.FilePath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("checkpoint restore remove %s: %w", cp.FilePath, removeErr)
			}
			continue
		}

		// Restore from backup.
		if cp.SnapshotPath == "" {
			continue
		}
		data, readErr := os.ReadFile(cp.SnapshotPath)
		if readErr != nil {
			return fmt.Errorf("checkpoint restore read snap %s: %w", cp.SnapshotPath, readErr)
		}

		// Ensure parent directory exists.
		if mkErr := os.MkdirAll(filepath.Dir(cp.FilePath), 0o755); mkErr != nil {
			return fmt.Errorf("checkpoint restore mkdir: %w", mkErr)
		}

		if writeErr := fileop.WriteFileAtomic(cp.FilePath, data, cp.FileMode); writeErr != nil {
			return fmt.Errorf("checkpoint restore write %s: %w", cp.FilePath, writeErr)
		}
	}
	return nil
}

// Prune removes old turns keeping only the most recent `keep` turns.
func (cs *checkpointStore) Prune(ctx context.Context, sessionID string, keep int) error {
	st, err := cs.getSession(sessionID)
	if err != nil {
		return err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.turnOrder) <= keep {
		return nil
	}

	toDelete := st.turnOrder[:len(st.turnOrder)-keep]
	deleteSet := make(map[string]struct{}, len(toDelete))
	for _, t := range toDelete {
		deleteSet[t] = struct{}{}
	}

	// Collect all SHA256 values that are still needed by surviving turns.
	neededSHA := make(map[string]struct{})
	for _, cp := range st.checkpoints {
		if _, pruned := deleteSet[cp.TurnID]; pruned {
			continue
		}
		if cp.SHA256 != "" {
			neededSHA[cp.SHA256] = struct{}{}
		}
	}

	// Delete .bak files and snapshot directories for pruned turns if their
	// content is not referenced by surviving turns.
	for _, t := range toDelete {
		// Remove snapshot directory if it exists.
		snapDir := cs.snapshotDir(sessionID, t)
		entries, readErr := os.ReadDir(snapDir)
		if readErr == nil {
			for _, e := range entries {
				sha := e.Name()[:min(8, len(e.Name()))]
				_ = sha
				// We need full sha to decide; resolve via checkpoints.
				fullPath := filepath.Join(snapDir, e.Name())
				canDelete := true
				for _, cp := range st.checkpoints {
					if _, pruned := deleteSet[cp.TurnID]; pruned {
						continue
					}
					if cp.SnapshotPath == fullPath {
						canDelete = false
						break
					}
				}
				if canDelete {
					_ = os.Remove(fullPath)
				}
			}
			_ = os.Remove(snapDir)
		}
	}

	// Rebuild in-memory state, removing pruned checkpoints.
	surviving := st.checkpoints[:0]
	for _, cp := range st.checkpoints {
		if _, pruned := deleteSet[cp.TurnID]; !pruned {
			surviving = append(surviving, cp)
		}
	}
	st.checkpoints = surviving
	st.turnOrder = st.turnOrder[len(toDelete):]

	// Rebuild sha256Index from surviving checkpoints.
	st.sha256Index = make(map[string]string, len(surviving))
	for _, cp := range surviving {
		if cp.SHA256 != "" && cp.SnapshotPath != "" {
			st.sha256Index[cp.SHA256] = cp.SnapshotPath
		}
	}

	// Rewrite manifest from surviving checkpoints.
	return cs.rewriteManifest(sessionID, st.checkpoints)
}

// rewriteManifest rewrites the manifest file with the given checkpoints.
// Caller must hold st.mu.
func (cs *checkpointStore) rewriteManifest(sessionID string, checkpoints []FileCheckpoint) error {
	dir := filepath.Join(cs.rootDir, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpPath := cs.manifestPath(sessionID) + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("checkpoint rewrite manifest create: %w", err)
	}

	w := bufio.NewWriter(f)
	for _, cp := range checkpoints {
		data, marshalErr := json.Marshal(cp)
		if marshalErr != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return marshalErr
		}
		_, _ = w.Write(data)
		_ = w.WriteByte('\n')
	}
	if err2 := w.Flush(); err2 != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err2
	}
	if err2 := f.Close(); err2 != nil {
		_ = os.Remove(tmpPath)
		return err2
	}

	return os.Rename(tmpPath, cs.manifestPath(sessionID))
}

// ─── helpers ──────────────────────────────────────────────────────────────────

var idCounter uint64

func generateID() string {
	// Use current time + a counter for uniqueness.
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), incIDCounter())
}

var idMu sync.Mutex
var idVal uint64

func incIDCounter() uint64 {
	idMu.Lock()
	defer idMu.Unlock()
	idVal++
	return idVal
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sortedTurns returns turns in a deterministic order based on their first
// checkpoint's CreatedAt, used internally for Prune ordering validation.
func sortedTurns(checkpoints []FileCheckpoint) []string {
	firstSeen := make(map[string]time.Time)
	for _, cp := range checkpoints {
		if t, ok := firstSeen[cp.TurnID]; !ok || cp.CreatedAt.Before(t) {
			firstSeen[cp.TurnID] = cp.CreatedAt
		}
	}
	turns := make([]string, 0, len(firstSeen))
	for t := range firstSeen {
		turns = append(turns, t)
	}
	sort.Slice(turns, func(i, j int) bool {
		return firstSeen[turns[i]].Before(firstSeen[turns[j]])
	})
	return turns
}

// Ensure io is used (used via io.Copy below in potential future usage).
var _ = io.Discard
