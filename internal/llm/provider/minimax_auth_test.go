package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestMiniMaxBearerUsesAuthorizationHeader(t *testing.T) {
	var gotAuth, gotXAPI, gotAnthropicVersion string
	var gotTemperature float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXAPI = r.Header.Get("x-api-key")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		var body struct {
			Temperature float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotTemperature = body.Temperature
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"request_id":"req-bearer"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(
		models.ProviderMiniMax,
		WithAPIKey("secret-minimax-key"),
		WithBaseURL(srv.URL+"/anthropic"),
		WithProviderAuthMode("bearer"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	_, _ = p.SendMessages(context.Background(), []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}, nil)
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("Authorization=%q, want Bearer prefix", gotAuth)
	}
	if gotXAPI != "" {
		t.Fatalf("x-api-key=%q, want empty", gotXAPI)
	}
	if gotAnthropicVersion == "" {
		t.Fatal("expected anthropic-version header")
	}
	if gotTemperature != 1 {
		t.Fatalf("temperature=%v, want 1 for MiniMax", gotTemperature)
	}
}

func TestMiniMaxCustomBaseDefaultUsesXAPIKeyHeader(t *testing.T) {
	var gotAuth, gotXAPI, gotAnthropicVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXAPI = r.Header.Get("x-api-key")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"request_id":"req-xapi"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(
		models.ProviderMiniMax,
		WithAPIKey("secret-minimax-key"),
		WithBaseURL(srv.URL+"/anthropic"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	_, _ = p.SendMessages(context.Background(), []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}, nil)
	if gotXAPI == "" {
		t.Fatal("expected x-api-key header")
	}
	if gotAuth != "" {
		t.Fatalf("authorization=%q, want empty", gotAuth)
	}
	if gotAnthropicVersion == "" {
		t.Fatal("expected anthropic-version header")
	}
}

func TestNativeAnthropicIgnoresMiniMaxAuthModeOption(t *testing.T) {
	var gotAuth, gotXAPI, gotAnthropicVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXAPI = r.Header.Get("x-api-key")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"request_id":"req-anthropic"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(
		models.ProviderAnthropic,
		WithAPIKey("secret-anthropic-key"),
		WithBaseURL(srv.URL),
		WithProviderAuthMode("bearer"),
		WithModel(models.SupportedModels[models.Claude4Sonnet]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	_, _ = p.SendMessages(context.Background(), []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}, nil)
	if gotXAPI == "" {
		t.Fatal("expected x-api-key header")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization=%q, want empty", gotAuth)
	}
	if gotAnthropicVersion == "" {
		t.Fatal("expected anthropic-version header")
	}
}

func TestMiniMax401HintContainsContextAndNoRawKey(t *testing.T) {
	const rawKey = "very-secret-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req-401-1")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key very-secret-key"}}`))
	}))
	defer srv.Close()

	p, err := NewProvider(
		models.ProviderMiniMax,
		WithAPIKey(rawKey),
		WithBaseURL(srv.URL+"/anthropic"),
		WithProviderProfile("token-plan"),
		WithProviderAuthMode("anthropic_x_api_key"),
		WithAuthSource("env"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	_, err = p.SendMessages(context.Background(), []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"provider=minimax",
		"model=MiniMax-M2.7",
		"baseURL=",
		"auth_source=env",
		"auth_mode=anthropic_x_api_key",
		"request_id=req-401-1",
		"recommendation=",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, rawKey) {
		t.Fatalf("error leaked raw key: %q", msg)
	}
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected wrapped anthropic error, got %T", err)
	}
	if !strings.Contains(strings.ToLower(msg), "token plan") || !strings.Contains(strings.ToLower(msg), "bearer") {
		t.Fatalf("expected token-plan recommendation, got %q", msg)
	}
}

func TestMiniMax401HintForStreamEventError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req-stream-401")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
	}))
	defer srv.Close()

	p, err := NewProvider(
		models.ProviderMiniMax,
		WithAPIKey("stream-secret"),
		WithBaseURL(srv.URL+"/anthropic"),
		WithProviderAuthMode("anthropic_x_api_key"),
		WithAuthSource("config"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	ch := p.StreamResponse(context.Background(), []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}, nil)
	for ev := range ch {
		if ev.Type == EventError {
			if ev.Error == nil {
				t.Fatal("expected stream error")
			}
			msg := ev.Error.Error()
			if !strings.Contains(msg, "provider=minimax") || !strings.Contains(msg, "request_id=req-stream-401") {
				t.Fatalf("unexpected stream error: %q", msg)
			}
			return
		}
	}
	t.Fatal("expected EventError from stream")
}
