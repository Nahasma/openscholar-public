package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultScholarTimeout = 30 * time.Second
	defaultScholarUA      = "OpenScholar/2.0"
	defaultAbstractMax    = 300
	defaultAuthorMax      = 3
	scholarCacheTTL       = 20 * time.Second
	scholarRetryMax       = 2
)

type scholarHTTPError struct {
	Message             string
	ErrorKind           string
	StatusCode          int
	RetryAfterMs        int64
	CooldownUntilUnixMs int64
	Attempts            int
	RetryCount          int
}

func (e *scholarHTTPError) Error() string { return e.Message }

type scholarRequestMeta struct {
	Source   string
	Action   string
	QueryKey string
	CacheHit bool
	Attempts int
}

type scholarCacheEntry struct {
	body      []byte
	expiresAt time.Time
}

type scholarPolicyResult struct {
	body []byte
	meta map[string]any
}

var (
	scholarHTTPStateMu sync.Mutex
	scholarCooldownBy  = map[string]time.Time{}
	scholarCacheByKey  = map[string]scholarCacheEntry{}
	scholarFlights     singleflight.Group
)

func scholarBuildQueryKey(source, action string, parts map[string]string) string {
	keys := []string{"action", "source", "query", "id", "offset", "limit", "filter"}
	values := []string{action, source}
	for _, k := range keys[2:] {
		values = append(values, parts[k])
	}
	return strings.Join(values, "|")
}

func scholarFetchWithPolicy(ctx context.Context, rawURL string, headers map[string]string, meta scholarRequestMeta) ([]byte, map[string]any, error) {
	return scholarDoWithPolicy(ctx, http.MethodGet, rawURL, nil, headers, meta)
}

func scholarPostWithPolicy(ctx context.Context, rawURL string, jsonBody []byte, headers map[string]string, meta scholarRequestMeta) ([]byte, map[string]any, error) {
	return scholarDoWithPolicy(ctx, http.MethodPost, rawURL, jsonBody, headers, meta)
}

func scholarDoWithPolicy(ctx context.Context, method, rawURL string, requestBody []byte, headers map[string]string, meta scholarRequestMeta) ([]byte, map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultScholarTimeout)
	defer cancel()

	now := time.Now()
	scholarHTTPStateMu.Lock()
	scholarPruneExpiredCacheLocked(now)
	if entry, ok := scholarCacheByKey[meta.QueryKey]; ok && entry.expiresAt.After(now) {
		body := append([]byte(nil), entry.body...)
		scholarHTTPStateMu.Unlock()
		return body, map[string]any{
			"source":      meta.Source,
			"action":      meta.Action,
			"query_key":   meta.QueryKey,
			"cache_hit":   true,
			"attempts":    0,
			"retry_count": 0,
		}, nil
	}
	if until := scholarCooldownBy[meta.Source]; until.After(now) {
		scholarHTTPStateMu.Unlock()
		return nil, map[string]any{
				"source":                 meta.Source,
				"action":                 meta.Action,
				"query_key":              meta.QueryKey,
				"cache_hit":              false,
				"attempts":               0,
				"retry_count":            0,
				"error_kind":             "provider_cooldown",
				"retry_after_ms":         maxInt64(0, time.Until(until).Milliseconds()),
				"cooldown_until_unix_ms": until.UnixMilli(),
			}, &scholarHTTPError{
				Message:             "source cooldown active",
				ErrorKind:           "provider_cooldown",
				RetryAfterMs:        maxInt64(0, time.Until(until).Milliseconds()),
				CooldownUntilUnixMs: until.UnixMilli(),
			}
	}
	scholarHTTPStateMu.Unlock()

	flightValue, err, shared := scholarFlights.Do(meta.QueryKey, func() (any, error) {
		body, resultMeta, err := scholarDoHTTPRequestWithPolicy(ctx, method, rawURL, requestBody, headers, meta)
		return scholarPolicyResult{body: body, meta: resultMeta}, err
	})
	result, _ := flightValue.(scholarPolicyResult)
	if shared && result.meta != nil {
		result.meta = scholarCopyMetadata(result.meta)
		result.meta["inflight_shared"] = true
	}
	return result.body, result.meta, err
}

func scholarDoHTTPRequestWithPolicy(ctx context.Context, method, rawURL string, requestBody []byte, headers map[string]string, meta scholarRequestMeta) ([]byte, map[string]any, error) {
	attempts := 0
	retryCount := 0
	for attempt := 0; attempt <= scholarRetryMax; attempt++ {
		attempts = attempt + 1
		var bodyReader io.Reader
		if requestBody != nil {
			bodyReader = bytes.NewReader(requestBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
		if err != nil {
			return nil, nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", defaultScholarUA)
		if requestBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := scholarHTTPClient.Do(req)
		if err != nil {
			if scholarRetryableNetErr(err) && attempt < scholarRetryMax {
				retryCount++
				time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
				continue
			}
			return nil, map[string]any{
					"source":      meta.Source,
					"action":      meta.Action,
					"query_key":   meta.QueryKey,
					"cache_hit":   false,
					"attempts":    attempts,
					"retry_count": retryCount,
					"error_kind":  "temporary_failure",
				}, &scholarHTTPError{
					Message:    fmt.Sprintf("request failed: %v", err),
					ErrorKind:  "temporary_failure",
					Attempts:   attempts,
					RetryCount: retryCount,
				}
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if attempt < scholarRetryMax {
				retryCount++
				time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
				continue
			}
			return nil, map[string]any{
					"source":      meta.Source,
					"action":      meta.Action,
					"query_key":   meta.QueryKey,
					"cache_hit":   false,
					"attempts":    attempts,
					"retry_count": retryCount,
					"error_kind":  "temporary_failure",
				}, &scholarHTTPError{
					Message:    fmt.Sprintf("read response: %v", readErr),
					ErrorKind:  "temporary_failure",
					StatusCode: resp.StatusCode,
					Attempts:   attempts,
					RetryCount: retryCount,
				}
		}

		if resp.StatusCode == http.StatusOK {
			scholarHTTPStateMu.Lock()
			scholarCacheByKey[meta.QueryKey] = scholarCacheEntry{
				body:      append([]byte(nil), body...),
				expiresAt: time.Now().Add(scholarCacheTTL),
			}
			scholarHTTPStateMu.Unlock()
			return body, map[string]any{
				"source":      meta.Source,
				"action":      meta.Action,
				"query_key":   meta.QueryKey,
				"cache_hit":   false,
				"attempts":    attempts,
				"retry_count": retryCount,
			}, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			if retryAfter <= 0 {
				retryAfter = 10 * time.Second
			}
			until := time.Now().Add(retryAfter)
			scholarHTTPStateMu.Lock()
			scholarCooldownBy[meta.Source] = until
			scholarHTTPStateMu.Unlock()
			return nil, map[string]any{
					"source":                 meta.Source,
					"action":                 meta.Action,
					"query_key":              meta.QueryKey,
					"cache_hit":              false,
					"attempts":               attempts,
					"retry_count":            retryCount,
					"error_kind":             "rate_limited",
					"retry_after_ms":         retryAfter.Milliseconds(),
					"cooldown_until_unix_ms": until.UnixMilli(),
				}, &scholarHTTPError{
					Message:             "rate limited",
					ErrorKind:           "rate_limited",
					StatusCode:          resp.StatusCode,
					RetryAfterMs:        retryAfter.Milliseconds(),
					CooldownUntilUnixMs: until.UnixMilli(),
					Attempts:            attempts,
					RetryCount:          retryCount,
				}
		}
		if resp.StatusCode >= 500 && attempt < scholarRetryMax {
			retryCount++
			time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
			continue
		}
		errorKind := "non_retryable"
		if resp.StatusCode >= 500 {
			errorKind = "temporary_failure"
		}
		return nil, map[string]any{
				"source":      meta.Source,
				"action":      meta.Action,
				"query_key":   meta.QueryKey,
				"cache_hit":   false,
				"attempts":    attempts,
				"retry_count": retryCount,
				"error_kind":  errorKind,
			}, &scholarHTTPError{
				Message:    fmt.Sprintf("API returned status %d: %s", resp.StatusCode, truncateStr(string(body), 200)),
				ErrorKind:  errorKind,
				StatusCode: resp.StatusCode,
				Attempts:   attempts,
				RetryCount: retryCount,
			}
	}
	return nil, nil, errors.New("unexpected: exhausted retries")
}

func scholarCopyMetadata(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta)+1)
	for k, v := range meta {
		out[k] = v
	}
	return out
}

// scholarFetch performs an HTTP GET with timeout and standard error handling.
// Returns the response body. Non-200 status codes return an error containing
// the status code and a truncated body snippet.
func scholarFetch(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	body, _, err := scholarFetchWithPolicy(ctx, rawURL, headers, scholarRequestMeta{
		Source:   "generic",
		Action:   "fetch",
		QueryKey: scholarBuildQueryKey("generic", "fetch", map[string]string{"filter": rawURL}),
	})
	return body, err
}

// scholarPost performs an HTTP POST with a JSON body, timeout, and standard
// error handling. Returns the response body.
func scholarPost(ctx context.Context, rawURL string, jsonBody []byte, headers map[string]string) ([]byte, error) {
	body, _, err := scholarPostWithPolicy(ctx, rawURL, jsonBody, headers, scholarRequestMeta{
		Source:   "generic",
		Action:   "post",
		QueryKey: scholarBuildQueryKey("generic", "post", map[string]string{"filter": rawURL + ":" + string(jsonBody)}),
	})
	return body, err
}

func scholarSearchRequestMeta(source, query string, offset, limit int, filter string) scholarRequestMeta {
	return scholarRequestMeta{
		Source: source,
		Action: "search",
		QueryKey: scholarBuildQueryKey(source, "search", map[string]string{
			"query":  query,
			"offset": fmt.Sprintf("%d", offset),
			"limit":  fmt.Sprintf("%d", limit),
			"filter": filter,
		}),
	}
}

func scholarCacheGet(source, action, queryKey string) ([]byte, map[string]any, bool) {
	now := time.Now()
	scholarHTTPStateMu.Lock()
	defer scholarHTTPStateMu.Unlock()
	scholarPruneExpiredCacheLocked(now)
	entry, ok := scholarCacheByKey[queryKey]
	if !ok || !entry.expiresAt.After(now) {
		return nil, nil, false
	}
	return append([]byte(nil), entry.body...), map[string]any{
		"source":      source,
		"action":      action,
		"query_key":   queryKey,
		"cache_hit":   true,
		"attempts":    0,
		"retry_count": 0,
	}, true
}

func scholarCacheSet(queryKey string, body []byte) {
	scholarHTTPStateMu.Lock()
	defer scholarHTTPStateMu.Unlock()
	scholarPruneExpiredCacheLocked(time.Now())
	scholarCacheByKey[queryKey] = scholarCacheEntry{
		body:      append([]byte(nil), body...),
		expiresAt: time.Now().Add(scholarCacheTTL),
	}
}

func scholarPruneExpiredCacheLocked(now time.Time) {
	for key, entry := range scholarCacheByKey {
		if !entry.expiresAt.After(now) {
			delete(scholarCacheByKey, key)
		}
	}
}

// scholarFetchRaw is like scholarFetch but returns the raw response without
// checking the status code, so the caller can handle specific status codes.
func scholarFetchRaw(ctx context.Context, rawURL string, headers map[string]string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultScholarTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", defaultScholarUA)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := scholarHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}

	return resp.StatusCode, body, nil
}

func scholarRetryableNetErr(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var tlsErr *tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "tls") || strings.Contains(msg, "connection reset")
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func scholarErrorMetadata(meta map[string]any, err error) map[string]any {
	out := map[string]any{}
	for k, v := range meta {
		out[k] = v
	}
	var shErr *scholarHTTPError
	if errors.As(err, &shErr) {
		if out["error_kind"] == nil {
			out["error_kind"] = shErr.ErrorKind
		}
		if shErr.RetryAfterMs > 0 {
			out["retry_after_ms"] = shErr.RetryAfterMs
		}
		if shErr.CooldownUntilUnixMs > 0 {
			out["cooldown_until_unix_ms"] = shErr.CooldownUntilUnixMs
		}
		if shErr.Attempts > 0 && out["attempts"] == nil {
			out["attempts"] = shErr.Attempts
		}
		if out["retry_count"] == nil {
			out["retry_count"] = shErr.RetryCount
		}
		if shErr.StatusCode > 0 && out["status_code"] == nil {
			out["status_code"] = shErr.StatusCode
		}
	}
	return out
}

func scholarParseArxivIDQuery(query string) string {
	raw := strings.TrimSpace(query)
	raw = strings.TrimPrefix(strings.ToLower(raw), "arxiv:")
	raw = strings.TrimPrefix(raw, "arxiv/")
	if raw == "" {
		return ""
	}
	idCore := raw
	if idx := strings.LastIndex(raw, "v"); idx > 0 {
		version := raw[idx+1:]
		if version == "" {
			return ""
		}
		if _, err := strconv.Atoi(version); err != nil {
			return ""
		}
		idCore = raw[:idx]
	}
	if len(idCore) < 10 {
		return ""
	}
	if idCore[4] != '.' {
		return ""
	}
	prefix := idCore[:4]
	suffix := idCore[5:]
	if _, err := strconv.Atoi(prefix); err != nil {
		return ""
	}
	if _, err := strconv.Atoi(suffix); err != nil {
		return ""
	}
	return raw
}

// scholarFormatAuthors formats an author slice for display, truncating to
// defaultAuthorMax authors and appending a count suffix when there are more.
func scholarFormatAuthors(authors []string) string {
	if len(authors) == 0 {
		return ""
	}
	if len(authors) <= defaultAuthorMax {
		return strings.Join(authors, "; ")
	}
	return strings.Join(authors[:defaultAuthorMax], "; ") + fmt.Sprintf(" ... (%d authors)", len(authors))
}

// scholarTruncateAbstract truncates an abstract to defaultAbstractMax chars.
func scholarTruncateAbstract(s string) string {
	if len(s) <= defaultAbstractMax {
		return s
	}
	return s[:defaultAbstractMax] + "..."
}

// scholarWriteBibHeader writes the opening @type{key, line into sb.
func scholarWriteBibHeader(sb *strings.Builder, entryType, citeKey string) {
	fmt.Fprintf(sb, "\nBibTeX:\n@%s{%s,\n", entryType, citeKey)
}
