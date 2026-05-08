package web

import (
	"fmt"
	"time"
)

// SearchErrorKind 分类搜索错误，Router 据此决定是否轮转。
type SearchErrorKind string

const (
	SearchErrQuotaExhausted SearchErrorKind = "quota_exhausted" // 429 或本地 quota 用完
	SearchErrRateLimited    SearchErrorKind = "rate_limited"    // 429 Retry-After
	SearchErrTemporary      SearchErrorKind = "temporary"       // 5xx / timeout / 网络错误
	SearchErrAuth           SearchErrorKind = "auth"            // 401 / 403 / key 无效
	SearchErrNoResults      SearchErrorKind = "no_results"      // 搜索成功但无结果
)

// SearchError 是结构化的搜索错误。
type SearchError struct {
	Provider   string          // backend 名称
	Kind       SearchErrorKind // 错误类型
	RetryAfter time.Duration   // 429 → 尊重 Retry-After header
	Cause      error           // 底层错误
}

func (e *SearchError) Error() string {
	return fmt.Sprintf("search %s [%s]: %v", e.Provider, e.Kind, e.Cause)
}

func (e *SearchError) Unwrap() error { return e.Cause }

// ShouldRotate 返回 true 表示 Router 应切换到下一个 backend。
func (e *SearchError) ShouldRotate() bool {
	switch e.Kind {
	case SearchErrQuotaExhausted, SearchErrRateLimited, SearchErrTemporary, SearchErrNoResults:
		return true
	default:
		return false // auth 等错误不应轮转
	}
}

// ProviderAttempt 记录一次搜索尝试，用于调试可观测性。
type ProviderAttempt struct {
	Name     string  `json:"name"`
	Success  bool    `json:"success"`
	Duration float64 `json:"duration_seconds"`
	Error    string  `json:"error,omitempty"`
}

// SearchFailureError is returned by SearchRouter when all backends fail or are skipped.
// It preserves provider-level attempts for observability and optionally wraps the last backend error.
type SearchFailureError struct {
	Message  string
	Attempts []ProviderAttempt
	Cause    error
}

func (e *SearchFailureError) Error() string {
	if e == nil {
		return "search failed"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *SearchFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
