package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fetch retrieves a URL, converts to markdown, summarises with a small model,
// and caches the result. Aligned with CC's WebFetchTool.call().
func (r *Runtime) Fetch(ctx context.Context, rawURL, prompt string) (*FetchResult, *RedirectResult, error) {
	safeURL := RedactURL(rawURL)
	proxy := newProxyResolver(r.config.Proxy, r.urlPolicy.resolver)
	policyMeta := PolicyMetadata{ProxyMode: proxy.mode()}

	// 1. Validate URL (including SSRF checks)
	if r.config.URLPolicy.BlockURLSecrets && ContainsURLSecret(rawURL) {
		return nil, nil, fmt.Errorf("invalid URL: URL contains sensitive query parameters")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("URL parse: %w", err)
	}
	policyURL := rawURL
	policyParsed := *parsedURL
	if strings.ToLower(policyParsed.Scheme) == "http" {
		policyParsed.Scheme = "https"
		policyURL = policyParsed.String()
	}
	proxyReq := &http.Request{URL: &policyParsed}
	if _, resolvedMeta, proxyErr := proxy.decide(proxyReq); proxyErr != nil {
		return nil, nil, &PolicyError{
			Message:  proxyErr.Error(),
			Metadata: blockedMeta(resolvedMeta, "", "blocked_proxy_endpoint", false),
		}
	} else {
		policyMeta = resolvedMeta
	}
	if err := r.urlPolicy.CheckWithOptions(ctx, policyURL, URLCheckOptions{
		PolicyMetadata: policyMeta,
		FakeIPCIDRs:    r.config.Proxy.FakeIPCIDRs,
		Metadata:       &policyMeta,
	}); err != nil {
		return nil, nil, fmt.Errorf("invalid URL: %w", err)
	}
	if err := r.websitePolicy.Check(parsedURL.Hostname()); err != nil {
		return nil, nil, err
	}

	// 2. Check cache
	cacheKey := CanonicalCacheURL(safeURL)
	if entry := r.cache.Get(cacheKey); entry != nil {
		if entry.Code >= 400 {
			return nil, nil, fetchHTTPStatusError(entry.Code, entry.CodeText, safeURL)
		}
		// Cache hit — still run the LLM summarisation on the cached content
		result, err := r.summariseContent(ctx, safeURL, entry.Content, entry.ContentType, prompt, entry.Bytes, entry.Code, entry.CodeText)
		if err != nil {
			return nil, nil, err
		}
		result.CacheHit = true
		result.Policy = policyMeta
		return result, nil, nil
	}

	// 3. Upgrade http → https (aligned with CC)
	parsed := *parsedURL
	fetchURL := rawURL
	if strings.ToLower(parsed.Scheme) == "http" {
		parsed.Scheme = "https"
		fetchURL = parsed.String()
	}

	// 4. HTTP GET with redirect handling
	body, code, codeText, contentType, redirectResult, err := r.httpGet(ctx, safeURL, fetchURL)
	if err != nil {
		return nil, nil, err
	}
	if redirectResult != nil {
		return nil, redirectResult, nil
	}

	bodyLen := len(body)

	if code >= 400 {
		return nil, nil, fetchHTTPStatusError(code, codeText, safeURL)
	}

	// 5. Convert HTML → Markdown
	content := body
	if strings.Contains(contentType, "text/html") {
		md, err := HTMLToMarkdown(body)
		if err == nil {
			content = md
		}
		// If conversion fails, use raw content
	}

	// 6. Cache the raw content (before truncation/summarisation)
	if !(r.config.URLPolicy.SkipCacheWhenHasSecret && ContainsURLSecret(rawURL)) {
		r.cache.Set(cacheKey, &CacheEntry{
			Content:     content,
			ContentType: contentType,
			CodeText:    codeText,
			Bytes:       bodyLen,
			Code:        code,
			Size:        len(content),
		})
	}

	// 7. Summarise
	result, err := r.summariseContent(ctx, safeURL, content, contentType, prompt, bodyLen, code, codeText)
	if err != nil {
		return nil, nil, err
	}
	result.Policy = policyMeta
	return result, nil, nil
}

// httpGet performs the HTTP GET with redirect policy aligned with CC.
func (r *Runtime) httpGet(ctx context.Context, originalURL, fetchURL string) (body string, code int, codeText string, contentType string, redirect *RedirectResult, err error) {
	maxBytes := int64(MaxContentLength)
	if r.config.FetchMaxResponseMB > 0 {
		maxBytes = int64(r.config.FetchMaxResponseMB) * 1024 * 1024
	}

	redirectCount := 0
	currentURL := fetchURL

	client := &http.Client{
		Timeout:   FetchTimeout,
		Transport: newPolicyRoundTripper(r.urlPolicy, r.websitePolicy, r.config, PurposeFetch),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Always stop the default redirect handler — we manage redirects manually.
			return http.ErrUseLastResponse
		},
	}

	for {
		if redirectCount > MaxRedirects {
			return "", 0, "", "", nil, fmt.Errorf("too many redirects (exceeded %d)", MaxRedirects)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return "", 0, "", "", nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "text/markdown, text/html, */*")
		req.Header.Set("User-Agent", "OpenScholar/2.0")

		resp, err := client.Do(req)
		if err != nil {
			return "", 0, "", "", nil, classifyFetchTransportError(err, currentURL)
		}

		// Handle redirects
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			resp.Body.Close()
			location := resp.Header.Get("Location")
			if location == "" {
				return "", resp.StatusCode, http.StatusText(resp.StatusCode), "", nil, fmt.Errorf("redirect missing Location header")
			}

			// Resolve relative redirect URLs
			redirectURL, err := url.Parse(location)
			if err != nil {
				return "", resp.StatusCode, http.StatusText(resp.StatusCode), "", nil, fmt.Errorf("invalid redirect URL: %w", err)
			}
			base, _ := url.Parse(currentURL)
			resolvedURL := base.ResolveReference(redirectURL).String()
			if err := r.validateURLForPolicies(ctx, resolvedURL, PurposeRedirect); err != nil {
				return "", resp.StatusCode, http.StatusText(resp.StatusCode), "", nil, err
			}

			if IsPermittedRedirect(currentURL, resolvedURL) {
				// Same-host redirect — follow it
				currentURL = resolvedURL
				redirectCount++
				continue
			}

			// Cross-host redirect — return redirect info for the LLM to decide
			return "", 0, "", "", &RedirectResult{
				OriginalURL: originalURL,
				RedirectURL: resolvedURL,
				StatusCode:  resp.StatusCode,
			}, nil
		}

		// Read body with size limit
		ct := resp.Header.Get("Content-Type")
		reader := io.LimitReader(resp.Body, maxBytes+1)
		bodyBytes, readErr := io.ReadAll(reader)
		resp.Body.Close()
		if readErr != nil {
			return "", resp.StatusCode, resp.Status, ct, nil, classifyFetchBodyReadError(readErr, resp.StatusCode, currentURL)
		}
		if int64(len(bodyBytes)) > maxBytes {
			return "", resp.StatusCode, resp.Status, ct, nil, &FetchError{
				Kind:        FetchErrTooLarge,
				Message:     fmt.Sprintf("response body exceeds %dMB limit", r.config.FetchMaxResponseMB),
				StatusCode:  resp.StatusCode,
				Provider:    "http",
				Source:      currentURL,
				Recoverable: false,
			}
		}

		return string(bodyBytes), resp.StatusCode, http.StatusText(resp.StatusCode), ct, nil, nil
	}
}

func classifyFetchBodyReadError(err error, statusCode int, sourceURL string) *FetchError {
	kind := FetchErrRead
	message := "read response body failed"
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(strings.ToLower(err.Error()), "eof") {
		kind = FetchErrEOF
		message = "read response body hit EOF"
	}
	return &FetchError{
		Kind:        kind,
		Message:     message,
		StatusCode:  statusCode,
		Provider:    "http",
		Source:      sourceURL,
		Recoverable: true,
		Cause:       err,
	}
}

func fetchHTTPStatusError(code int, codeText, sourceURL string) *FetchError {
	kind := FetchErrHTTPStatus
	recoverable := code >= 500
	switch code {
	case http.StatusNotFound:
		kind = FetchErrNotFound
	case http.StatusForbidden:
		kind = FetchErrForbidden
	default:
		if code >= 500 {
			kind = FetchErrServer
		}
	}
	return &FetchError{
		Kind:        kind,
		Message:     fmt.Sprintf("HTTP %d %s", code, codeText),
		StatusCode:  code,
		Provider:    "http",
		Source:      sourceURL,
		Recoverable: recoverable,
	}
}

func classifyFetchTransportError(err error, sourceURL string) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "eof") {
		return &FetchError{Kind: FetchErrEOF, Message: "transport EOF", Provider: "http", Source: sourceURL, Recoverable: true, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &FetchError{Kind: FetchErrTimeout, Message: "request timeout", Provider: "http", Source: sourceURL, Recoverable: true, Cause: err}
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) || strings.Contains(msg, "tls") || strings.Contains(msg, "x509") {
		return &FetchError{Kind: FetchErrTLS, Message: "tls handshake failed", Provider: "http", Source: sourceURL, Recoverable: true, Cause: err}
	}
	return &FetchError{Kind: FetchErrRead, Message: "transport request failed", Provider: "http", Source: sourceURL, Recoverable: true, Cause: err}
}

func (r *Runtime) validateURLForPolicies(ctx context.Context, rawURL string, purpose URLPurpose) error {
	if r.config.URLPolicy.BlockURLSecrets && ContainsURLSecret(rawURL) {
		return fmt.Errorf("unsafe redirect URL: URL contains sensitive parameters")
	}
	proxy := newProxyResolver(r.config.Proxy, r.urlPolicy.resolver)
	policyMeta := PolicyMetadata{ProxyMode: proxy.mode()}
	if purpose != PurposeRedirect {
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid redirect URL: %w", err)
		}
		_, resolvedMeta, proxyErr := proxy.decide(&http.Request{URL: u})
		if proxyErr != nil {
			return &PolicyError{
				Message:  proxyErr.Error(),
				Metadata: blockedMeta(resolvedMeta, "", "blocked_proxy_endpoint", false),
			}
		}
		policyMeta = resolvedMeta
	}
	if err := r.urlPolicy.CheckWithOptions(ctx, rawURL, URLCheckOptions{
		PolicyMetadata: policyMeta,
		FakeIPCIDRs:    r.config.Proxy.FakeIPCIDRs,
	}); err != nil {
		return fmt.Errorf("unsafe redirect URL: %w", err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %w", err)
	}
	if err := r.websitePolicy.Check(u.Hostname()); err != nil {
		return err
	}
	return nil
}

// summariseContent truncates and optionally runs the LLM summariser.
func (r *Runtime) summariseContent(ctx context.Context, rawURL, content, contentType, prompt string, bodyBytes, code int, codeText string) (*FetchResult, error) {
	start := time.Now()

	maxLen := r.config.FetchMaxMarkdownLen
	if maxLen <= 0 {
		maxLen = MaxMarkdownLength
	}

	isPreapproved := false
	if parsed, err := url.Parse(rawURL); err == nil {
		isPreapproved = IsPreapprovedHost(parsed.Hostname(), parsed.Path)
	}

	// For pre-approved markdown content within length limit, return raw.
	if isPreapproved && strings.Contains(contentType, "text/markdown") && len(content) < maxLen {
		return &FetchResult{
			Content:     content,
			Bytes:       bodyBytes,
			Code:        code,
			CodeText:    codeText,
			ContentType: contentType,
			Duration:    time.Since(start).Seconds(),
			URL:         rawURL,
		}, nil
	}

	// Truncate
	truncated := TruncateContent(content, maxLen)

	// Build secondary model prompt
	modelPrompt := MakeSecondaryModelPrompt(truncated, prompt, isPreapproved)

	// Call LLM for summarisation
	var result string
	if r.callLLM != nil {
		var err error
		result, err = r.callLLM(ctx, modelPrompt)
		if err != nil {
			// Fallback: return truncated content directly
			result = truncated
		} else if containsPseudoToolCall(result) {
			return nil, &FetchError{
				Kind:        FetchErrValidation,
				Message:     "secondary summarizer output failed validation",
				StatusCode:  code,
				Provider:    "llm",
				Source:      rawURL,
				Recoverable: false,
			}
		}
	} else {
		result = truncated
	}

	return &FetchResult{
		Content:     result,
		Bytes:       bodyBytes,
		Code:        code,
		CodeText:    codeText,
		ContentType: contentType,
		Duration:    time.Since(start).Seconds(),
		URL:         rawURL,
	}, nil
}

func containsPseudoToolCall(s string) bool {
	trimmed := strings.TrimSpace(s)
	text := strings.ToLower(trimmed)
	if text == "" {
		return false
	}
	if strings.Contains(text, "<invoke") ||
		strings.Contains(text, "<tool_call") ||
		strings.Contains(text, "<function_call") ||
		strings.Contains(text, "<tool_use") {
		return true
	}

	for _, candidate := range pseudoToolJSONCandidates(trimmed) {
		var raw any
		if err := json.Unmarshal([]byte(candidate), &raw); err == nil && hasPseudoToolCallJSON(raw) {
			return true
		}
	}
	return false
}

func pseudoToolJSONCandidates(text string) []string {
	var candidates []string
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			candidates = append(candidates, candidate)
		}
	}

	add(text)
	for _, block := range fencedCodeBlockCandidates(text) {
		add(block)
	}
	for _, candidate := range balancedJSONCandidates(text, '{', '}') {
		add(candidate)
	}
	for _, candidate := range balancedJSONCandidates(text, '[', ']') {
		add(candidate)
	}
	return candidates
}

func fencedCodeBlockCandidates(text string) []string {
	var blocks []string
	rest := text
	for {
		start := strings.Index(rest, "```")
		if start < 0 {
			return blocks
		}
		afterFence := rest[start+3:]
		lineEnd := strings.IndexByte(afterFence, '\n')
		if lineEnd < 0 {
			return blocks
		}
		contentStart := lineEnd + 1
		close := strings.Index(afterFence[contentStart:], "```")
		if close < 0 {
			return blocks
		}
		blocks = append(blocks, afterFence[contentStart:contentStart+close])
		rest = afterFence[contentStart+close+3:]
	}
}

func balancedJSONCandidates(text string, open, close byte) []string {
	var candidates []string
	for start := 0; start < len(text); start++ {
		if text[start] != open {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(text); i++ {
			ch := text[i]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			if ch == '"' {
				inString = true
				continue
			}
			if ch == open {
				depth++
			}
			if ch == close {
				depth--
				if depth == 0 {
					candidates = append(candidates, text[start:i+1])
					break
				}
			}
		}
	}
	return candidates
}

func hasPseudoToolCallJSON(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			lk := strings.ToLower(strings.TrimSpace(k))
			if lk == "tool_call" || lk == "tool_calls" || lk == "function_call" || lk == "function_calls" {
				return true
			}
			if hasPseudoToolCallJSON(child) {
				return true
			}
		}
	case []any:
		for _, item := range x {
			if hasPseudoToolCallJSON(item) {
				return true
			}
		}
	}
	return false
}

// Search delegates to the search provider.
func (r *Runtime) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	if r.search == nil {
		return nil, fmt.Errorf("no web search provider configured")
	}
	if opts.MaxUses <= 0 {
		opts.MaxUses = r.config.SearchMaxUses
	}
	result, err := r.search.SearchWeb(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	result.Policy = PolicyMetadata{
		ProxyMode: newProxyResolver(r.config.Proxy, r.urlPolicy.resolver).mode(),
	}
	filtered := make([]SearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		if strings.TrimSpace(hit.URL) == "" {
			continue
		}
		if r.config.URLPolicy.BlockURLSecrets && ContainsURLSecret(hit.URL) {
			continue
		}
		proxy := newProxyResolver(r.config.Proxy, r.urlPolicy.resolver)
		policyMeta := PolicyMetadata{ProxyMode: proxy.mode()}
		u, err := url.Parse(hit.URL)
		if err != nil {
			continue
		}
		if _, resolvedMeta, proxyErr := proxy.decide(&http.Request{URL: u}); proxyErr != nil {
			continue
		} else {
			policyMeta = resolvedMeta
		}
		if err := r.urlPolicy.CheckWithOptions(ctx, hit.URL, URLCheckOptions{
			PolicyMetadata: policyMeta,
			FakeIPCIDRs:    r.config.Proxy.FakeIPCIDRs,
		}); err != nil {
			continue
		}
		if err := r.websitePolicy.Check(u.Hostname()); err != nil {
			continue
		}
		if result.Policy.PolicyDecision == "" {
			result.Policy = policyMeta
		}
		hit.URL = RedactURL(hit.URL)
		filtered = append(filtered, hit)
	}
	result.Hits = filtered
	return result, nil
}
