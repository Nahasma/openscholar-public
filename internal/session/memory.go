package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// MemoryTrigger 触发条件配置
type MemoryTrigger struct {
	MinInitTokens          int // 首次触发最低 token 数（默认 10000）
	MinDeltaTokens         int // 增量触发最低新增 token（默认 5000）
	MinToolCallsSinceWrite int // 自上次摘要后最少工具调用次数（默认 5）
	MaxFileBytes           int // notes 文件最大字节（默认 4096）
}

// DefaultMemoryTrigger returns default trigger thresholds.
func DefaultMemoryTrigger() MemoryTrigger {
	return MemoryTrigger{
		MinInitTokens:          10000,
		MinDeltaTokens:         5000,
		MinToolCallsSinceWrite: 5,
		MaxFileBytes:           4096,
	}
}

// RollingMemory 显式状态（解决增量判定问题）
type RollingMemory struct {
	SessionID        string
	FilePath         string // .openscholar/session-memory/<sessionID>/notes.md
	LastTokenCount   int    // 上次触发时的 input token 总量
	LastToolCallSeen int    // 上次触发时的 tool call 累计次数
	UpdatedAt        time.Time
}

// SessionMemoryQuery 用于执行后台 LLM 查询，替代直接依赖 ForkedRunner
type SessionMemoryQuery func(ctx context.Context, prompt string) (string, error)

// MemoryManager Session Memory 管理器
type MemoryManager interface {
	// ShouldTrigger 检查是否应触发摘要更新
	ShouldTrigger(sessionID string, currentTokens, currentToolCalls int) bool

	// UpdateNotes 执行摘要更新（通过回调 query 调用 LLM）
	UpdateNotes(ctx context.Context, sessionID string, recentTurns string, query SessionMemoryQuery) error

	// UpdateNotesWithState performs UpdateNotes and advances the RollingMemory state.
	UpdateNotesWithState(ctx context.Context, sessionID string, recentTurns string, currentTokens, currentToolCalls int, query SessionMemoryQuery) error

	// LoadForPrompt 加载 session notes 用于 prompt 注入
	LoadForPrompt(ctx context.Context, sessionID string) (string, error)
}

const memoryPromptTemplate = `You are a session memory assistant. Analyze the recent conversation and update the running summary notes. Focus on:
- Key decisions made and their rationale
- Files created/modified and why
- Important findings or discoveries
- Pending tasks or open questions

Current notes (update these, keep under 500 words):
---
%s
---

Recent conversation (last 5 turns):
%s

Output ONLY the updated notes, no explanations.`

const (
	notesMetaPrefix = "<!-- openscholar-session-memory:"
	notesMetaSuffix = "-->"
)

type persistedMemoryState struct {
	LastTokenCount   int   `json:"last_token_count"`
	LastToolCallSeen int   `json:"last_tool_call_seen"`
	UpdatedAtUnix    int64 `json:"updated_at_unix"`
}

type memoryManager struct {
	mu      sync.Mutex
	dataDir string
	cfg     MemoryTrigger
	states  map[string]*RollingMemory
}

// NewMemoryManager creates a new MemoryManager.
func NewMemoryManager(dataDir string, cfg MemoryTrigger) MemoryManager {
	return &memoryManager{
		dataDir: dataDir,
		cfg:     cfg,
		states:  make(map[string]*RollingMemory),
	}
}

func (m *memoryManager) notesPath(sessionID string) string {
	return filepath.Join(m.dataDir, "session-memory", sessionID, "notes.md")
}

func (m *memoryManager) getState(sessionID string) *RollingMemory {
	state, ok := m.states[sessionID]
	if !ok {
		state = &RollingMemory{
			SessionID: sessionID,
			FilePath:  m.notesPath(sessionID),
		}
		if persisted, err := m.loadPersistedState(state.FilePath); err == nil && persisted != nil {
			state.LastTokenCount = persisted.LastTokenCount
			state.LastToolCallSeen = persisted.LastToolCallSeen
			if persisted.UpdatedAtUnix > 0 {
				state.UpdatedAt = time.Unix(persisted.UpdatedAtUnix, 0)
			}
		}
		m.states[sessionID] = state
	}
	return state
}

// ShouldTrigger checks whether a memory summary update should be triggered.
func (m *memoryManager) ShouldTrigger(sessionID string, currentTokens, currentToolCalls int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getState(sessionID)

	if state.LastTokenCount == 0 && state.LastToolCallSeen == 0 {
		// First trigger check
		return currentTokens >= m.cfg.MinInitTokens && currentToolCalls >= m.cfg.MinToolCallsSinceWrite
	}

	// Incremental trigger check
	deltaTokens := currentTokens - state.LastTokenCount
	deltaToolCalls := currentToolCalls - state.LastToolCallSeen
	return deltaTokens >= m.cfg.MinDeltaTokens && deltaToolCalls >= m.cfg.MinToolCallsSinceWrite
}

// UpdateNotesWithState performs a memory summary update and advances the RollingMemory state.
func (m *memoryManager) UpdateNotesWithState(ctx context.Context, sessionID string, recentTurns string, currentTokens, currentToolCalls int, query SessionMemoryQuery) error {
	if err := m.UpdateNotes(ctx, sessionID, recentTurns, query); err != nil {
		return err
	}
	// Fix 2: advance RollingMemory after successful write
	m.mu.Lock()
	state := m.getState(sessionID)
	state.LastTokenCount = currentTokens
	state.LastToolCallSeen = currentToolCalls
	state.UpdatedAt = time.Now()
	meta := &persistedMemoryState{
		LastTokenCount:   state.LastTokenCount,
		LastToolCallSeen: state.LastToolCallSeen,
		UpdatedAtUnix:    state.UpdatedAt.Unix(),
	}
	m.mu.Unlock()
	notes, err := m.LoadForPrompt(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := m.writeNotesFile(m.notesPath(sessionID), notes, meta); err != nil {
		return err
	}
	return nil
}

// UpdateNotes performs a memory summary update using the provided LLM query callback.
func (m *memoryManager) UpdateNotes(ctx context.Context, sessionID string, recentTurns string, query SessionMemoryQuery) error {
	// Read existing notes
	notesPath := m.notesPath(sessionID)
	existingNotes := ""
	data, err := os.ReadFile(notesPath)
	if err == nil {
		_, existingNotes = parseNotesFile(data)
	}

	// Build prompt
	prompt := fmt.Sprintf(memoryPromptTemplate, existingNotes, recentTurns)

	// Call LLM
	updatedNotes, err := query(ctx, prompt)
	if err != nil {
		return fmt.Errorf("session memory query failed: %w", err)
	}

	// Truncate if over MaxFileBytes
	if len(updatedNotes) > m.cfg.MaxFileBytes {
		updatedNotes = updatedNotes[:m.cfg.MaxFileBytes]
	}

	if err := m.writeNotesFile(notesPath, updatedNotes, nil); err != nil {
		return fmt.Errorf("failed to write session notes: %w", err)
	}

	return nil
}

// LoadForPrompt loads session notes for prompt injection. Returns empty string if file doesn't exist.
func (m *memoryManager) LoadForPrompt(_ context.Context, sessionID string) (string, error) {
	notesPath := m.notesPath(sessionID)
	data, err := os.ReadFile(notesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read session notes: %w", err)
	}
	_, notes := parseNotesFile(data)
	return notes, nil
}

func (m *memoryManager) loadPersistedState(path string) (*persistedMemoryState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	meta, _ := parseNotesFile(data)
	return meta, nil
}

func (m *memoryManager) writeNotesFile(path string, notes string, meta *persistedMemoryState) error {
	if meta == nil {
		if existing, err := m.loadPersistedState(path); err == nil && existing != nil {
			meta = existing
		}
	}

	content := notes
	if meta != nil {
		payload, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		content = notesMetaPrefix + string(payload) + notesMetaSuffix + "\n" + notes
	}
	return fileop.SafeWrite(path, []byte(content), fileop.WithMkdir())
}

func parseNotesFile(data []byte) (*persistedMemoryState, string) {
	raw := string(data)
	if !strings.HasPrefix(raw, notesMetaPrefix) {
		return nil, raw
	}
	end := strings.Index(raw, notesMetaSuffix)
	if end < 0 {
		return nil, raw
	}
	metaJSON := strings.TrimSpace(strings.TrimPrefix(raw[:end], notesMetaPrefix))
	var meta persistedMemoryState
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return nil, raw
	}
	notes := strings.TrimPrefix(raw[end+len(notesMetaSuffix):], "\n")
	return &meta, notes
}
