package web

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// QuotaConfig 描述 backend 的配额限制。
type QuotaConfig struct {
	MonthlyLimit int `json:"monthlyLimit"` // 0 = 无限
	DailyLimit   int `json:"dailyLimit"`   // 0 = 无日限
}

// ProviderState 单个 backend 的运行时状态。
type ProviderState struct {
	UsedToday   int       `json:"usedToday"`
	UsedMonth   int       `json:"usedMonth"`
	DayKey      string    `json:"dayKey"`               // "2026-04-10"
	MonthKey    string    `json:"monthKey"`              // "2026-04"
	CooldownEnd time.Time `json:"cooldownEnd,omitempty"` // 429 冷却截止
	LastError   string    `json:"lastError,omitempty"`
}

// SearchState 管理所有 backend 的用量和健康状态。
type SearchState struct {
	mu        sync.Mutex
	providers map[string]*ProviderState
	filePath  string // 持久化路径: .openscholar/web-search-state.json
}

// NewSearchState 创建状态管理器。filePath 为空则纯内存模式。
func NewSearchState(filePath string) *SearchState {
	return &SearchState{
		providers: make(map[string]*ProviderState),
		filePath:  filePath,
	}
}

// Load 从 JSON 文件加载状态。文件不存在时返回空状态(nil error)。
func (s *SearchState) Load() error {
	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var loaded map[string]*ProviderState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	s.providers = loaded
	return nil
}

// Save 将状态写入 JSON 文件。原子写入 (tmp → rename)。filePath 为空时跳过。
func (s *SearchState) Save() error {
	if s.filePath == "" {
		return nil
	}

	s.mu.Lock()
	data, err := json.Marshal(s.providers)
	s.mu.Unlock()

	if err != nil {
		return err
	}

	return fileop.WriteFileAtomic(s.filePath, data, 0644)
}

// HasRemaining 检查 backend 是否还有 quota。自动重置过期计数器。
// quota.MonthlyLimit=0 或 DailyLimit=0 表示无限。
func (s *SearchState) HasRemaining(name string, quota QuotaConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.getOrCreate(name)
	now := time.Now().UTC()

	// Reset daily counter if day changed.
	dayKey := now.Format("2006-01-02")
	if ps.DayKey != dayKey {
		ps.UsedToday = 0
		ps.DayKey = dayKey
	}

	// Reset monthly counter if month changed.
	monthKey := now.Format("2006-01")
	if ps.MonthKey != monthKey {
		ps.UsedMonth = 0
		ps.MonthKey = monthKey
	}

	// Check daily limit.
	if quota.DailyLimit > 0 && ps.UsedToday >= quota.DailyLimit {
		return false
	}

	// Check monthly limit.
	if quota.MonthlyLimit > 0 && ps.UsedMonth >= quota.MonthlyLimit {
		return false
	}

	return true
}

// InCooldown 检查 backend 是否在冷却期。
func (s *SearchState) InCooldown(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.getOrCreate(name)
	if ps.CooldownEnd.IsZero() {
		return false
	}
	return time.Now().UTC().Before(ps.CooldownEnd)
}

// Reserve atomically checks quota and increments usage in one step.
// Returns true if reservation succeeded (quota was available).
// This prevents concurrent requests from both seeing "available" before either increments.
func (s *SearchState) Reserve(name string, quota QuotaConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.getOrCreate(name)
	now := time.Now().UTC()

	// Auto-reset counters on boundary crossing.
	dayKey := now.Format("2006-01-02")
	if ps.DayKey != dayKey {
		ps.UsedToday = 0
		ps.DayKey = dayKey
	}
	monthKey := now.Format("2006-01")
	if ps.MonthKey != monthKey {
		ps.UsedMonth = 0
		ps.MonthKey = monthKey
	}

	// Check limits
	if quota.DailyLimit > 0 && ps.UsedToday >= quota.DailyLimit {
		return false
	}
	if quota.MonthlyLimit > 0 && ps.UsedMonth >= quota.MonthlyLimit {
		return false
	}

	// Reserve: increment immediately
	ps.UsedToday++
	ps.UsedMonth++
	return true
}

// RecordSuccess clears cooldown and last error for a backend.
func (s *SearchState) RecordSuccess(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.getOrCreate(name)
	ps.CooldownEnd = time.Time{} // clear cooldown
	ps.LastError = ""
}

// SetCooldown 设置 backend 的冷却期。
func (s *SearchState) SetCooldown(name string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.getOrCreate(name)
	ps.CooldownEnd = time.Now().UTC().Add(duration)
}

// Stats 返回所有 backend 的用量统计副本（供 TUI/日志使用）。
func (s *SearchState) Stats() map[string]ProviderState {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]ProviderState, len(s.providers))
	for k, v := range s.providers {
		result[k] = *v
	}
	return result
}

// getOrCreate 获取或创建 ProviderState（必须在持有 mu 的情况下调用）。
func (s *SearchState) getOrCreate(name string) *ProviderState {
	if ps, ok := s.providers[name]; ok {
		return ps
	}
	ps := &ProviderState{}
	s.providers[name] = ps
	return ps
}
