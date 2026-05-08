package kb

import (
	"errors"
	"testing"
)

func TestParseScriptOutput(t *testing.T) {
	t.Run("ok true success", func(t *testing.T) {
		out := []byte(`{"ok":true,"tree":{"doc_name":"x","structure":[{"node_id":"n1","title":"t","start_index":1,"end_index":1}]},"total_pages":3,"total_tokens":10,"model_used":"simple_extract"}`)
		res, err := parseScriptOutput(out, "")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if res.Tree.DocName != "x" || res.TotalPages != 3 || res.ModelUsed != "simple_extract" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("ok false structured error", func(t *testing.T) {
		out := []byte(`{"ok":false,"error_type":"pdf_open_error","error_message":"cannot open","retryable":false}`)
		_, err := parseScriptOutput(out, "stderr")
		var pErr *PageIndexError
		if !errors.As(err, &pErr) {
			t.Fatalf("expected PageIndexError, got %T %v", err, err)
		}
		if pErr.Type != "pdf_open_error" || pErr.Message != "cannot open" {
			t.Fatalf("unexpected PageIndexError: %+v", pErr)
		}
	})

	t.Run("legacy success without ok but with tree", func(t *testing.T) {
		out := []byte(`{"tree":{"doc_name":"legacy","structure":[{"node_id":"n1","title":"t","start_index":1,"end_index":1}]}}`)
		res, err := parseScriptOutput(out, "")
		if err != nil {
			t.Fatalf("expected legacy success, got error: %v", err)
		}
		if res.Tree.DocName != "legacy" {
			t.Fatalf("unexpected doc name: %q", res.Tree.DocName)
		}
	})

	t.Run("missing ok and tree fails", func(t *testing.T) {
		out := []byte(`{"total_pages":2}`)
		_, err := parseScriptOutput(out, "stderr")
		if err == nil {
			t.Fatal("expected error for missing ok/tree")
		}
	})
}
