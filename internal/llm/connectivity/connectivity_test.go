package connectivity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestDiscoverModels_OpenAICompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
	}))
	defer srv.Close()

	res := DiscoverModels(context.Background(), TestRequest{
		Provider: models.ProviderOpenAICompatible,
		BaseURL:  srv.URL + "/v1",
	})
	if !res.ProviderOK || len(res.Models) != 1 || res.Models[0].ID != "model-a" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDiscoverModels_OllamaUsesTagsPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path = %q, want /api/tags", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:14b"}]}`))
	}))
	defer srv.Close()

	res := DiscoverModels(context.Background(), TestRequest{
		Provider: models.ProviderOllama,
		BaseURL:  srv.URL + "/v1",
	})
	if !res.ProviderOK || len(res.Models) != 1 || res.Models[0].ID != "qwen2.5:14b" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDiscoverModels_SiliconFlowAddsChatFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "text" {
			t.Fatalf("type query = %q, want text", got)
		}
		if got := r.URL.Query().Get("sub_type"); got != "chat" {
			t.Fatalf("sub_type query = %q, want chat", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"sf-chat-a","type":"text","sub_type":"chat"},{"id":"sf-image","type":"image"}]}`))
	}))
	defer srv.Close()

	res := DiscoverModels(context.Background(), TestRequest{
		Provider: models.ProviderSiliconFlow,
		BaseURL:  srv.URL + "/v1",
	})
	if !res.ProviderOK || len(res.Models) != 1 || res.Models[0].ID != "sf-chat-a" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if _, ok := models.SupportedModels[models.ModelID("sf-chat-a")]; ok {
		t.Fatal("discovered model should not mutate curated supported models")
	}
}

func TestDiscoverModels_AnthropicCompatibleDisplayName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/models" {
			t.Fatalf("path = %q, want /anthropic/v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"MiniMax-M2.7","display_name":"MiniMax M2.7"}]}`))
	}))
	defer srv.Close()

	res := DiscoverModels(context.Background(), TestRequest{
		Provider: models.ProviderMiniMax,
		BaseURL:  srv.URL + "/anthropic",
	})
	if !res.ProviderOK || len(res.Models) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Models[0].Name != "MiniMax M2.7" {
		t.Fatalf("model name = %q, want display_name", res.Models[0].Name)
	}
}

func TestPingChat_ClassifiesStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		category ErrorCategory
		warning  bool
	}{
		{"auth", http.StatusUnauthorized, `{"error":"bad key"}`, ErrorAuth, false},
		{"missing model", http.StatusNotFound, `{"error":"model not found"}`, ErrorModelNotFound, false},
		{"rate limit", http.StatusTooManyRequests, `{"error":"slow down"}`, ErrorRateLimit, true},
		{"quota", http.StatusPaymentRequired, `{"error":"insufficient quota"}`, ErrorQuota, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			res := PingChat(context.Background(), TestRequest{
				Provider: models.ProviderOpenAICompatible,
				ModelID:  "model-a",
				BaseURL:  srv.URL + "/v1",
			})
			if res.Category != tt.category || res.Warning != tt.warning {
				t.Fatalf("unexpected result: %+v", res)
			}
		})
	}
}

func TestPingChat_RedactsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`secret-key`))
	}))
	defer srv.Close()

	res := PingChat(context.Background(), TestRequest{
		Provider: models.ProviderOpenAICompatible,
		ModelID:  "model-a",
		APIKey:   "secret-key",
		BaseURL:  srv.URL + "/v1",
	})
	if res.Message == "secret-key" {
		t.Fatalf("API key was not redacted: %+v", res)
	}
}

func TestPingChat_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	res := PingChat(context.Background(), TestRequest{
		Provider: models.ProviderOpenAICompatible,
		ModelID:  "model-a",
		BaseURL:  srv.URL + "/v1",
		Timeout:  1 * time.Millisecond,
	})
	if res.Category != ErrorTimeout {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestProbeChat_MiniMaxAuthModesAndRequestID(t *testing.T) {
	tests := []struct {
		name             string
		basePath         string
		mode             ProbeAuthMode
		wantXAPI         bool
		wantBearer       bool
		wantAnthropicVer bool
		requestIDHeader  bool
	}{
		{name: "legacy x-api-key", basePath: "/anthropic", mode: ProbeAuthModeAnthropicXAPIKey, wantXAPI: true, wantAnthropicVer: true},
		{name: "legacy bearer", basePath: "/anthropic", mode: ProbeAuthModeBearer, wantBearer: true, wantAnthropicVer: true, requestIDHeader: true},
		{name: "token-plan x-api-key", basePath: "/anthropic", mode: ProbeAuthModeAnthropicXAPIKey, wantXAPI: true, wantAnthropicVer: true},
		{name: "token-plan bearer", basePath: "/anthropic", mode: ProbeAuthModeBearer, wantBearer: true, wantAnthropicVer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("x-api-key"); (got != "") != tt.wantXAPI {
					t.Fatalf("x-api-key present=%v, want %v", got != "", tt.wantXAPI)
				}
				if got := r.Header.Get("Authorization"); (got != "") != tt.wantBearer {
					t.Fatalf("authorization present=%v, want %v", got != "", tt.wantBearer)
				}
				if tt.wantBearer && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					t.Fatalf("authorization=%q, want Bearer prefix", r.Header.Get("Authorization"))
				}
				if got := r.Header.Get("anthropic-version"); (got != "") != tt.wantAnthropicVer {
					t.Fatalf("anthropic-version present=%v, want %v", got != "", tt.wantAnthropicVer)
				}
				if tt.requestIDHeader {
					w.Header().Set("x-request-id", "req-hdr-1")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key secret-key"}}`))
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"request_id":"req-body-1","error":{"type":"authentication_error","message":"bad key secret-key"}}`))
			}))
			defer srv.Close()

			res := ProbeChat(context.Background(), ProbeRequest{
				TestRequest: TestRequest{
					Provider: models.ProviderMiniMax,
					ModelID:  "MiniMax-M2.7",
					APIKey:   "secret-key",
					BaseURL:  srv.URL + tt.basePath,
				},
				AuthMode: tt.mode,
			})
			if res.HTTPStatus != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401", res.HTTPStatus)
			}
			if res.ErrorType != "authentication_error" {
				t.Fatalf("error type=%q", res.ErrorType)
			}
			if strings.Contains(res.Message, "secret-key") {
				t.Fatalf("message leaked key: %q", res.Message)
			}
			wantReqID := "req-body-1"
			if tt.requestIDHeader {
				wantReqID = "req-hdr-1"
			}
			if res.RequestID != wantReqID {
				t.Fatalf("request id=%q, want %q", res.RequestID, wantReqID)
			}
		})
	}
}

func TestPingChat_CustomMiniMaxDefaultUsesXAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Fatal("expected x-api-key header")
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("did not expect authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer srv.Close()

	res := PingChat(context.Background(), TestRequest{
		Provider: models.ProviderMiniMax,
		ModelID:  "MiniMax-M2.7",
		APIKey:   "secret-key",
		BaseURL:  srv.URL + "/anthropic",
	})
	if res.HTTPStatus != http.StatusOK || !res.ProviderOK || !res.ModelOK {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestApplyAuth_DefaultMiniMaxTokenPlanUsesBearer(t *testing.T) {
	spec, _ := models.ProviderSpecByID(models.ProviderMiniMax)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	applyAuth(req, spec, "secret-key", ProbeAuthModeAuto, "https://api.minimaxi.com/anthropic", "", "")
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		t.Fatalf("authorization=%q, want bearer", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatalf("x-api-key should be empty, got %q", req.Header.Get("x-api-key"))
	}
}

func TestEndpointURL_MiniMaxProfileResolvesBaseURL(t *testing.T) {
	endpoint, _, err := endpointURL(TestRequest{
		Provider: models.ProviderMiniMax,
		Profile:  "token-plan",
	}, "/v1/models")
	if err != nil {
		t.Fatalf("endpointURL error: %v", err)
	}
	if !strings.HasPrefix(endpoint, "https://api.minimaxi.com/anthropic/") {
		t.Fatalf("endpoint=%q, want current MiniMax base", endpoint)
	}
}

func TestDiscoverModels_MiniMaxCustomBearerAuthMode(t *testing.T) {
	var gotAuth, gotXAPI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXAPI = r.Header.Get("x-api-key")
		_, _ = w.Write([]byte(`{"data":[{"id":"MiniMax-M2.7"}]}`))
	}))
	defer srv.Close()

	res := DiscoverModels(context.Background(), TestRequest{
		Provider: models.ProviderMiniMax,
		APIKey:   "secret-key",
		BaseURL:  srv.URL + "/anthropic",
		AuthMode: "bearer",
	})
	if !res.ProviderOK {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("authorization=%q, want bearer", gotAuth)
	}
	if gotXAPI != "" {
		t.Fatalf("x-api-key=%q, want empty", gotXAPI)
	}
}

func TestExtractRequestID_FromBodyNested(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":       "authentication_error",
			"message":    "bad",
			"request_id": "nested-req-id",
		},
	})
	id := extractRequestID(http.Header{}, body)
	if id != "nested-req-id" {
		t.Fatalf("request id=%q, want nested-req-id", id)
	}
}
