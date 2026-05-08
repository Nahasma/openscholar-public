package provider

import (
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestAnthropicShouldRetryStreamError(t *testing.T) {
	client := &anthropicClient{}
	api429Err := &anthropic.Error{StatusCode: 429}

	t.Run("retries retryable api errors before any events", func(t *testing.T) {
		retry, after, err := client.shouldRetryStreamError(1, false, api429Err)
		if err != nil {
			t.Fatalf("shouldRetryStreamError returned error: %v", err)
		}
		if !retry {
			t.Fatal("expected retry=true")
		}
		if after <= 0 {
			t.Fatalf("expected positive retry delay, got %d", after)
		}
	})

	t.Run("does not retry after events emitted", func(t *testing.T) {
		retry, after, err := client.shouldRetryStreamError(1, true, api429Err)
		if retry {
			t.Fatal("expected retry=false")
		}
		if after != 0 {
			t.Fatalf("expected retry delay 0, got %d", after)
		}
		if !errors.Is(err, api429Err) {
			t.Fatalf("expected original error, got %v", err)
		}
	})
}
