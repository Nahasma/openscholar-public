package initwizard

import (
	"context"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// ValidateResult holds the result of an API key validation.
type ValidateResult struct {
	Provider models.ModelProvider
	Valid    bool
	Error    string
	Latency time.Duration
}

// ValidateProviders validates all non-empty API keys in parallel.
// For each provider, sends max_tokens=1 "hi" to the cheapest model.
func ValidateProviders(keys map[models.ModelProvider]string, baseURLs map[models.ModelProvider]string) map[models.ModelProvider]ValidateResult {
	results := make(map[models.ModelProvider]ValidateResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for prov, key := range keys {
		if key == "" {
			continue
		}
		wg.Add(1)
		go func(p models.ModelProvider, k string) {
			defer wg.Done()
			result := validateProvider(p, k, baseURLs[p])
			mu.Lock()
			results[p] = result
			mu.Unlock()
		}(prov, key)
	}

	wg.Wait()
	return results
}

func validateProvider(provName models.ModelProvider, apiKey, baseURL string) ValidateResult {
	start := time.Now()

	cheapestID := models.CheapestModel(provName)
	if cheapestID == "" {
		// OpenAI Compatible — skip validation if no model defined
		return ValidateResult{
			Provider: provName,
			Valid:    true,
			Latency:  time.Since(start),
		}
	}

	model, ok := models.SupportedModels[cheapestID]
	if !ok {
		return ValidateResult{
			Provider: provName,
			Valid:    false,
			Error:    "cheapest model not found",
			Latency:  time.Since(start),
		}
	}

	opts := []provider.ProviderClientOption{
		provider.WithAPIKey(apiKey),
		provider.WithModel(model),
		provider.WithMaxTokens(1),
	}
	if baseURL != "" {
		opts = append(opts, provider.WithBaseURL(baseURL))
	}

	p, err := provider.NewProvider(provName, opts...)
	if err != nil {
		return ValidateResult{
			Provider: provName,
			Valid:    false,
			Error:    err.Error(),
			Latency:  time.Since(start),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs := []message.Message{
		{
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "hi"},
			},
		},
	}

	_, err = p.SendMessages(ctx, msgs, nil)
	latency := time.Since(start)

	if err != nil {
		errStr := err.Error()
		// Rate limit is still a successful auth
		if isRateLimitError(errStr) {
			return ValidateResult{Provider: provName, Valid: true, Latency: latency}
		}
		return ValidateResult{Provider: provName, Valid: false, Error: errStr, Latency: latency}
	}

	return ValidateResult{Provider: provName, Valid: true, Latency: latency}
}

func isRateLimitError(errStr string) bool {
	for _, s := range []string{"rate_limit", "rate limit", "429", "too many requests"} {
		if contains(errStr, s) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
