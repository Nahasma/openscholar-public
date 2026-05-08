package magicdoc

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/Nahasma/openscholar-public/internal/util"
)

// FileReadEvent 带 session 隔离的文件读取事件
type FileReadEvent struct {
	SessionID string
	FilePath  string
	Content   string    // 文件内容（用于 header 解析）
	ReadAt    time.Time
}

// UpdateQuery 用于后台 LLM 更新 MagicDoc，替代直接依赖 ForkedRunner
type UpdateQuery func(ctx context.Context, prompt string) (string, error)

// Service MagicDocs 服务
type Service interface {
	// OnFileRead View 工具成功读取后调用
	OnFileRead(evt FileReadEvent)

	// PendingUpdates 返回需要更新的 MagicDoc 列表
	PendingUpdates(sessionID string) []Document

	// RunUpdate 对指定 MagicDoc 执行更新
	RunUpdate(ctx context.Context, doc Document, query UpdateQuery) error

	// List 列出当前 session 的 MagicDocs
	List(sessionID string) []Document
}

// serviceContextKey context key for Service
type serviceContextKey struct{}

// ServiceCtxKey is the context key to retrieve a Service
var ServiceCtxKey = serviceContextKey{}

// ServiceFromContext 从 context 获取 Service
func ServiceFromContext(ctx context.Context) Service {
	svc, _ := ctx.Value(ServiceCtxKey).(Service)
	return svc
}

// serviceImpl is the concrete implementation of Service
type serviceImpl struct {
	registry          *Registry
	minUpdateInterval time.Duration
	seq               util.SequentialRunner[string] // Fix 6: per-file serialization

	// track last update time per session+path for rate limiting
	mu           sync.Mutex
	lastUpdate   map[string]time.Time // key: sessionID+":"+filePath
}

// NewService 创建新的 MagicDoc Service
func NewService(minUpdateInterval time.Duration) Service {
	return &serviceImpl{
		registry:          NewRegistry(),
		minUpdateInterval: minUpdateInterval,
		seq:               util.NewSequentialRunner[string](),
		lastUpdate:        make(map[string]time.Time),
	}
}

// OnFileRead View 工具成功读取后调用
func (s *serviceImpl) OnFileRead(evt FileReadEvent) {
	doc, ok := ParseHeader(evt.Content)
	if !ok {
		return
	}

	doc.SessionID = evt.SessionID
	doc.Path = evt.FilePath
	doc.LastReadAt = evt.ReadAt

	s.registry.Register(evt.SessionID, evt.FilePath, doc)

	// Update LastReadAt in registry (Register preserves LastUpdatedAt but sets LastReadAt via the doc)
	// Re-fetch and update LastReadAt explicitly
	if existing, found := s.registry.Get(evt.SessionID, evt.FilePath); found {
		existing.LastReadAt = evt.ReadAt
		s.registry.Register(evt.SessionID, evt.FilePath, existing)
	}
}

// PendingUpdates 返回需要更新的 MagicDoc 列表
// 条件: LastReadAt > LastUpdatedAt（读过但还没更新过）
func (s *serviceImpl) PendingUpdates(sessionID string) []Document {
	docs := s.registry.List(sessionID)
	var pending []Document
	for _, doc := range docs {
		if doc.LastReadAt.After(doc.LastUpdatedAt) {
			pending = append(pending, doc)
		}
	}
	return pending
}

// RunUpdate 对指定 MagicDoc 执行更新
// Fix 6: serialized per session:path to prevent concurrent read-modify-write races.
func (s *serviceImpl) RunUpdate(ctx context.Context, doc Document, query UpdateQuery) error {
	key := doc.SessionID + ":" + doc.Path
	return s.seq.Do(ctx, key, func(ctx context.Context) error {
		return s.runUpdateInner(ctx, doc, query)
	})
}

func (s *serviceImpl) runUpdateInner(ctx context.Context, doc Document, query UpdateQuery) error {
	// Check rate limit
	key := doc.SessionID + ":" + doc.Path
	s.mu.Lock()
	if last, ok := s.lastUpdate[key]; ok {
		if time.Since(last) < s.minUpdateInterval {
			s.mu.Unlock()
			return fmt.Errorf("magicdoc: update rate limited for %s (min interval %s)", doc.Path, s.minUpdateInterval)
		}
	}
	s.mu.Unlock()

	// Verify doc is registered (security: only edit known MagicDoc files)
	registered, found := s.registry.Get(doc.SessionID, doc.Path)
	if !found {
		return fmt.Errorf("magicdoc: file %s is not a registered MagicDoc for session %s", doc.Path, doc.SessionID)
	}

	// Read current file content
	currentBytes, err := os.ReadFile(doc.Path)
	if err != nil {
		return fmt.Errorf("magicdoc: failed to read file %s: %w", doc.Path, err)
	}
	currentContent := string(currentBytes)

	// Build prompt from instruction + current content
	instruction := registered.Instruction
	if instruction == "" {
		instruction = "Update this document to reflect the latest state."
	}
	prompt := fmt.Sprintf("%s\n\nCurrent file content:\n%s", instruction, currentContent)

	// Call LLM
	updatedContent, err := query(ctx, prompt)
	if err != nil {
		return fmt.Errorf("magicdoc: query failed for %s: %w", doc.Path, err)
	}

	// Diff check: skip if change is less than 10 chars
	diff := computeDiff(currentContent, updatedContent)
	if diff < 10 {
		return nil
	}

	// Write updated content
	if err := fileop.WriteFileAtomic(doc.Path, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("magicdoc: failed to write file %s: %w", doc.Path, err)
	}

	now := time.Now()

	// Update registry
	s.registry.MarkUpdated(doc.SessionID, doc.Path, now)

	// Update rate limit tracker
	s.mu.Lock()
	s.lastUpdate[key] = now
	s.mu.Unlock()

	return nil
}

// List 列出当前 session 的 MagicDocs
func (s *serviceImpl) List(sessionID string) []Document {
	return s.registry.List(sessionID)
}

// computeDiff returns a measure of change between two strings.
// Returns 0 if content is identical, otherwise the number of differing bytes.
// Fix 5: equal-length rewrites are no longer silently skipped.
func computeDiff(a, b string) int {
	if a == b {
		return 0
	}
	// Count differing bytes as a simple edit distance proxy
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	diffs := 0
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			diffs++
		}
	}
	// Add length difference
	lenDiff := len(a) - len(b)
	if lenDiff < 0 {
		lenDiff = -lenDiff
	}
	diffs += lenDiff
	if diffs == 0 {
		diffs = 1 // content differs but all sampled bytes match (unlikely edge)
	}
	return diffs
}
