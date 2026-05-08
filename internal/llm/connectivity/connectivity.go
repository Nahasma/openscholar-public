package connectivity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/minimax"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

type TestRequest struct {
	Provider models.ModelProvider
	ModelID  string
	APIKey   string
	BaseURL  string
	Profile  string
	AuthMode string
	Timeout  time.Duration
}

type ProbeAuthMode string

const (
	ProbeAuthModeAuto             ProbeAuthMode = ""
	ProbeAuthModeAnthropicXAPIKey ProbeAuthMode = "anthropic_x_api_key"
	ProbeAuthModeBearer           ProbeAuthMode = "bearer"
)

type ProbeRequest struct {
	TestRequest
	AuthMode ProbeAuthMode
}

type ErrorCategory string

const (
	ErrorAuth            ErrorCategory = "auth"
	ErrorBadBaseURL      ErrorCategory = "bad_base_url"
	ErrorModelNotFound   ErrorCategory = "model_not_found"
	ErrorRateLimit       ErrorCategory = "rate_limit"
	ErrorQuota           ErrorCategory = "quota"
	ErrorNetwork         ErrorCategory = "network"
	ErrorTimeout         ErrorCategory = "timeout"
	ErrorInvalidResponse ErrorCategory = "invalid_response"
	ErrorUnsupportedList ErrorCategory = "unsupported_list"
	ErrorAPI             ErrorCategory = "api"
	ErrorUnknown         ErrorCategory = "unknown"
)

type TestResult struct {
	ProviderOK bool
	ModelOK    bool
	Warning    bool
	Category   ErrorCategory
	Message    string
	HTTPStatus int
	ErrorType  string
	RequestID  string
	Suggestion string
	Latency    time.Duration
	Models     []models.Model
}

func DiscoverModels(ctx context.Context, req TestRequest) TestResult {
	start := time.Now()
	spec, ok := models.ProviderSpecByID(req.Provider)
	if !ok {
		return result(start, false, false, false, ErrorUnknown, fmt.Sprintf("unknown provider %q", req.Provider), "", 0, "", "")
	}
	if !spec.SupportsList || spec.Endpoints.ListPath == "" {
		return result(start, true, false, true, ErrorUnsupportedList, fmt.Sprintf("%s does not advertise model listing", spec.DisplayName), "", 0, "", "")
	}
	endpoint, suggestion, err := endpointURL(req, spec.Endpoints.ListPath)
	if err != nil {
		return result(start, false, false, false, ErrorBadBaseURL, Redact(err.Error(), req.APIKey), suggestion, 0, "", "")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result(start, false, false, false, ErrorBadBaseURL, Redact(err.Error(), req.APIKey), suggestion, 0, "", "")
	}
	applyAuth(httpReq, spec, req.APIKey, ProbeAuthModeAuto, req.BaseURL, req.Profile, req.AuthMode)
	if req.Provider == models.ProviderSiliconFlow {
		q := httpReq.URL.Query()
		if q.Get("type") == "" {
			q.Set("type", "text")
		}
		if q.Get("sub_type") == "" {
			q.Set("sub_type", "chat")
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	resp, err := httpClient(req).Do(httpReq)
	if err != nil {
		return resultForHTTPError(start, err, req.APIKey, suggestion)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	requestID := extractRequestID(resp.Header, body)
	if resp.StatusCode >= 400 {
		return resultForStatus(start, resp.StatusCode, body, req.APIKey, suggestion, requestID)
	}

	found, err := parseModelList(req.Provider, body)
	if err != nil {
		return result(start, true, false, true, ErrorInvalidResponse, Redact(err.Error(), req.APIKey), suggestion, resp.StatusCode, "", requestID)
	}
	return TestResult{ProviderOK: true, ModelOK: req.ModelID == "", Latency: time.Since(start), Models: found, Suggestion: suggestion, HTTPStatus: resp.StatusCode, RequestID: requestID}
}

func PingChat(ctx context.Context, req TestRequest) TestResult {
	return ProbeChat(ctx, ProbeRequest{TestRequest: req})
}

func ProbeChat(ctx context.Context, req ProbeRequest) TestResult {
	start := time.Now()
	spec, ok := models.ProviderSpecByID(req.TestRequest.Provider)
	if !ok {
		return result(start, false, false, false, ErrorUnknown, fmt.Sprintf("unknown provider %q", req.TestRequest.Provider), "", 0, "", "")
	}
	if strings.TrimSpace(req.TestRequest.ModelID) == "" {
		return result(start, false, false, false, ErrorModelNotFound, "model is required for connectivity test", "", 0, "", "")
	}
	endpoint, suggestion, err := endpointURL(req.TestRequest, spec.Endpoints.ChatPath)
	if err != nil {
		return result(start, false, false, false, ErrorBadBaseURL, Redact(err.Error(), req.TestRequest.APIKey), suggestion, 0, "", "")
	}

	body, err := chatPayload(spec, req.TestRequest.ModelID)
	if err != nil {
		return result(start, false, false, false, ErrorUnknown, err.Error(), suggestion, 0, "", "")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result(start, false, false, false, ErrorBadBaseURL, Redact(err.Error(), req.TestRequest.APIKey), suggestion, 0, "", "")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuth(httpReq, spec, req.TestRequest.APIKey, req.AuthMode, req.TestRequest.BaseURL, req.TestRequest.Profile, req.TestRequest.AuthMode)

	resp, err := httpClient(req.TestRequest).Do(httpReq)
	if err != nil {
		return resultForHTTPError(start, err, req.TestRequest.APIKey, suggestion)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	requestID := extractRequestID(resp.Header, respBody)
	if resp.StatusCode >= 400 {
		return resultForStatus(start, resp.StatusCode, respBody, req.TestRequest.APIKey, suggestion, requestID)
	}
	if !json.Valid(respBody) {
		return result(start, true, false, true, ErrorInvalidResponse, "provider returned invalid JSON", suggestion, resp.StatusCode, "", requestID)
	}
	return TestResult{ProviderOK: true, ModelOK: true, Latency: time.Since(start), Suggestion: suggestion, HTTPStatus: resp.StatusCode, RequestID: requestID}
}

func Redact(s string, secrets ...string) string {
	out := s
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			out = strings.ReplaceAll(out, secret, "[REDACTED]")
		}
	}
	if idx := strings.Index(strings.ToLower(out), "authorization:"); idx >= 0 {
		lineEnd := strings.IndexByte(out[idx:], '\n')
		if lineEnd < 0 {
			return out[:idx] + "Authorization: [REDACTED]"
		}
		return out[:idx] + "Authorization: [REDACTED]" + out[idx+lineEnd:]
	}
	return out
}

func NormalizeBaseURL(raw string) (normalized string, suggestion string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("base URL is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid base URL %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	normalized = parsed.String()
	if !strings.HasSuffix(parsed.Path, "/v1") && parsed.Path != "/v1" {
		suggestion = strings.TrimRight(normalized, "/") + "/v1"
	}
	return normalized, suggestion, nil
}

func endpointURL(req TestRequest, path string) (string, string, error) {
	base := strings.TrimSpace(req.BaseURL)
	if req.Provider == models.ProviderMiniMax {
		rt, err := minimax.ResolveRuntime(req.BaseURL, req.Profile, req.AuthMode)
		if err != nil {
			return "", "", err
		}
		base = rt.BaseURL
	} else if base == "" {
		base = models.ProviderDefaultBaseURL(req.Provider)
	}
	normalized, rawSuggestion, err := NormalizeBaseURL(base)
	if err != nil {
		return "", rawSuggestion, err
	}
	suggestion := ""
	if strings.TrimSpace(path) == "" {
		path = "/chat/completions"
	}
	if spec, ok := models.ProviderSpecByID(req.Provider); ok &&
		spec.Endpoints.RequiresV1 &&
		!strings.HasPrefix(path, "/v1") &&
		!strings.HasSuffix(strings.TrimRight(normalized, "/"), "/v1") &&
		(spec.Endpoints.DefaultBaseURL == "" || strings.HasSuffix(strings.TrimRight(spec.Endpoints.DefaultBaseURL, "/"), "/v1")) {
		suggestion = rawSuggestion
	}
	if req.Provider == models.ProviderOllama && path == "/api/tags" && strings.HasSuffix(normalized, "/v1") {
		normalized = strings.TrimSuffix(normalized, "/v1")
	}
	return strings.TrimRight(normalized, "/") + "/" + strings.TrimLeft(path, "/"), suggestion, nil
}

func httpClient(req TestRequest) *http.Client {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func applyAuth(req *http.Request, spec models.ProviderSpec, apiKey string, mode ProbeAuthMode, baseURL, profile, authMode string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}
	switch mode {
	case ProbeAuthModeAnthropicXAPIKey:
		req.Header.Set("x-api-key", apiKey)
		if spec.Kind == models.ProviderKindNativeAnthropic || spec.Kind == models.ProviderKindAnthropicCompat {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		return
	case ProbeAuthModeBearer:
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if spec.Kind == models.ProviderKindNativeAnthropic || spec.Kind == models.ProviderKindAnthropicCompat {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		return
	}
	if spec.ID == models.ProviderMiniMax {
		rt, err := minimax.ResolveRuntime(baseURL, profile, authMode)
		if err == nil && rt.Auth == minimax.AuthBearer {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
			return
		}
	}
	if spec.Kind == models.ProviderKindNativeAnthropic || spec.Kind == models.ProviderKindAnthropicCompat {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func chatPayload(spec models.ProviderSpec, modelID string) ([]byte, error) {
	if spec.Kind == models.ProviderKindNativeAnthropic || spec.Kind == models.ProviderKindAnthropicCompat {
		return json.Marshal(map[string]any{
			"model":      modelID,
			"max_tokens": 1,
			"messages": []map[string]string{{
				"role":    "user",
				"content": "ping",
			}},
		})
	}
	return json.Marshal(map[string]any{
		"model":      modelID,
		"max_tokens": 1,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "ping",
		}},
	})
}

func parseModelList(provider models.ModelProvider, body []byte) ([]models.Model, error) {
	var openAI struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			OwnedBy     string `json:"owned_by"`
			Type        string `json:"type"`
			SubType     string `json:"sub_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openAI); err == nil && len(openAI.Data) > 0 {
		out := make([]models.Model, 0, len(openAI.Data))
		for _, item := range openAI.Data {
			if item.ID == "" {
				continue
			}
			if provider == models.ProviderSiliconFlow {
				if item.Type != "" && item.Type != "text" {
					continue
				}
				if item.SubType != "" && item.SubType != "chat" {
					continue
				}
			}
			model := models.NewCustomModel(provider, item.ID)
			if item.Name != "" {
				model.Name = item.Name
			} else if item.DisplayName != "" {
				model.Name = item.DisplayName
			}
			out = append(out, model)
		}
		return out, nil
	}

	var ollama struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &ollama); err == nil && len(ollama.Models) > 0 {
		out := make([]models.Model, 0, len(ollama.Models))
		for _, item := range ollama.Models {
			if item.Name == "" {
				continue
			}
			out = append(out, models.NewCustomModel(provider, item.Name))
		}
		return out, nil
	}
	if !json.Valid(body) {
		return nil, errors.New("model list response is not valid JSON")
	}
	return nil, nil
}

func result(start time.Time, providerOK, modelOK, warning bool, category ErrorCategory, msg, suggestion string, status int, errType, requestID string) TestResult {
	return TestResult{
		ProviderOK: providerOK,
		ModelOK:    modelOK,
		Warning:    warning,
		Category:   category,
		Message:    msg,
		HTTPStatus: status,
		ErrorType:  errType,
		RequestID:  requestID,
		Suggestion: suggestion,
		Latency:    time.Since(start),
	}
}

func resultForHTTPError(start time.Time, err error, apiKey, suggestion string) TestResult {
	category := ErrorNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = ErrorTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		category = ErrorTimeout
	}
	return result(start, false, false, false, category, Redact(err.Error(), apiKey), suggestion, 0, "", "")
}

func resultForStatus(start time.Time, status int, body []byte, apiKey, suggestion, requestID string) TestResult {
	errType, message := providerErrorDetails(body, apiKey)
	if message == "" {
		message = Redact(strings.TrimSpace(string(body)), apiKey)
	}
	if message == "" {
		message = http.StatusText(status)
	}
	lower := strings.ToLower(message)

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return result(start, false, false, false, ErrorAuth, message, suggestion, status, errType, requestID)
	case status == http.StatusNotFound && strings.Contains(lower, "model"):
		return result(start, true, false, false, ErrorModelNotFound, message, suggestion, status, errType, requestID)
	case status == http.StatusNotFound:
		return result(start, false, false, true, ErrorUnsupportedList, message, suggestion, status, errType, requestID)
	case status == http.StatusTooManyRequests:
		return result(start, true, true, true, ErrorRateLimit, message, suggestion, status, errType, requestID)
	case strings.Contains(lower, "quota") || strings.Contains(lower, "insufficient"):
		return result(start, true, false, true, ErrorQuota, message, suggestion, status, errType, requestID)
	case status >= 500:
		return result(start, false, false, true, ErrorAPI, message, suggestion, status, errType, requestID)
	default:
		return result(start, false, false, false, ErrorAPI, message, suggestion, status, errType, requestID)
	}
}

func providerErrorDetails(body []byte, apiKey string) (string, string) {
	type nestedError struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	var payload struct {
		Type      string      `json:"type"`
		Message   string      `json:"message"`
		Error     nestedError `json:"error"`
		ErrorType string      `json:"error_type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	errorType := strings.TrimSpace(payload.Error.Type)
	if errorType == "" {
		errorType = strings.TrimSpace(payload.ErrorType)
	}
	if errorType == "" && strings.EqualFold(strings.TrimSpace(payload.Type), "error") {
		errorType = "error"
	}
	message := strings.TrimSpace(payload.Error.Message)
	if message == "" {
		message = strings.TrimSpace(payload.Message)
	}
	return Redact(errorType, apiKey), Redact(message, apiKey)
}

func extractRequestID(headers http.Header, body []byte) string {
	candidates := []string{"x-request-id", "request-id", "x-amzn-requestid", "x-amz-request-id"}
	for _, key := range candidates {
		if val := strings.TrimSpace(headers.Get(key)); val != "" {
			return val
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"request_id", "requestId", "req_id"} {
		if val, ok := payload[key].(string); ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		for _, key := range []string{"request_id", "requestId", "req_id"} {
			if val, ok := errObj[key].(string); ok {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func KeyFingerprint(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum[:4])
}
