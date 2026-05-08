package components

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/message"
)

var update = flag.Bool("update", false, "update golden files")

const goldenDir = "testdata"

func goldenPath(name string) string {
	return filepath.Join(goldenDir, name+".golden")
}

var chromaColorRe = regexp.MustCompile(`\x1b\[38;5;(24[0-9]|25[0-5])m`)

func normalizeChroma(s string) string {
	return chromaColorRe.ReplaceAllString(s, "\x1b[38;5;000m")
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	if *update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if diff := cmp.Diff(normalizeChroma(string(data)), normalizeChroma(got)); diff != "" {
		t.Errorf("golden mismatch for %s (-want +got):\n%s", name, diff)
	}
}

func fixtureSimpleChat() []message.Message {
	return []message.Message{
		{
			ID:   "user-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Hello, can you help me with my paper?"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:    "asst-1",
			Role:  message.Assistant,
			Model: models.ModelID("claude-3-5-sonnet-20241022"),
			Parts: []message.ContentPart{
				message.TextContent{Text: "Of course! I'd be happy to help you with your paper. What aspect would you like to work on first?"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "user-2",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "I need help with the introduction section."},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
}

func fixtureWithToolCalls() []message.Message {
	assistantMsg := message.Message{
		ID:    "asst-tool-1",
		Role:  message.Assistant,
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
		Parts: []message.ContentPart{
			message.TextContent{Text: "Let me check the current state of your file."},
			message.ToolCall{
				ID:       "tc-bash-1",
				Name:     "Bash",
				Input:    `{"command":"cat introduction.tex"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.ToolCall{
				ID:       "tc-edit-1",
				Name:     "Edit",
				Input:    `{"file_path":"introduction.tex","old_string":"placeholder","new_string":"This paper presents a novel approach"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.Finish{Reason: message.FinishReasonToolUse},
		},
	}
	toolResultMsg := message.Message{
		ID:   "tool-result-1",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc-bash-1",
				Name:       "Bash",
				Content:    "\\section{Introduction}\nThis is a placeholder text.",
			},
			message.ToolResult{
				ToolCallID: "tc-edit-1",
				Name:       "Edit",
				Content:    "File updated successfully.",
			},
		},
	}
	return []message.Message{assistantMsg, toolResultMsg}
}

func fixtureWithDiff() []message.Message {
	diffContent := `--- a/main.go
+++ b/main.go
@@ -1,7 +1,9 @@
 package main

 import (
+	"fmt"
 	"os"
 )

 func main() {
-	os.Exit(0)
+	fmt.Println("Hello, World!")
+	os.Exit(0)
 }`

	assistantMsg := message.Message{
		ID:    "asst-diff-1",
		Role:  message.Assistant,
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
		Parts: []message.ContentPart{
			message.TextContent{Text: "I've made the following changes to main.go:"},
			message.ToolCall{
				ID:       "tc-edit-diff-1",
				Name:     "Edit",
				Input:    `{"file_path":"main.go","old_string":"os.Exit(0)","new_string":"fmt.Println(\"Hello, World!\")\n\tos.Exit(0)"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.Finish{Reason: message.FinishReasonToolUse},
		},
	}
	toolResultMsg := message.Message{
		ID:   "tool-result-diff-1",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc-edit-diff-1",
				Name:       "Edit",
				Content:    diffContent,
			},
		},
	}
	return []message.Message{assistantMsg, toolResultMsg}
}

func fixtureStreaming() []message.Message {
	return []message.Message{
		{
			ID:   "user-stream-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Explain transformer architecture briefly."},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			// No Finish part — simulates a message still being streamed
			ID:    "asst-stream-1",
			Role:  message.Assistant,
			Model: models.ModelID("claude-3-5-sonnet-20241022"),
			Parts: []message.ContentPart{
				message.TextContent{Text: "The Transformer architecture, introduced in \"Attention is All You Need\", relies on self-attention mechanisms. The key components include"},
			},
		},
	}
}

func fixtureLongCode() []message.Message {
	longCode := "```go\npackage main\n\nimport (\n\t\"context\"\n\t\"fmt\"\n\t\"log\"\n\t\"net/http\"\n\t\"os\"\n\t\"sync\"\n\t\"time\"\n)\n\n// Server represents the HTTP server.\ntype Server struct {\n\tmu      sync.RWMutex\n\trouter  *http.ServeMux\n\taddr    string\n\tTimeout time.Duration\n}\n\n// NewServer creates a new Server instance.\nfunc NewServer(addr string) *Server {\n\treturn &Server{\n\t\trouter:  http.NewServeMux(),\n\t\taddr:    addr,\n\t\tTimeout: 30 * time.Second,\n\t}\n}\n\n// RegisterRoute adds a handler for the given pattern.\nfunc (s *Server) RegisterRoute(pattern string, handler http.HandlerFunc) {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\ts.router.HandleFunc(pattern, handler)\n}\n\n// Start begins accepting connections on the configured address.\nfunc (s *Server) Start(ctx context.Context) error {\n\tsrv := &http.Server{\n\t\tAddr:         s.addr,\n\t\tHandler:      s.router,\n\t\tReadTimeout:  s.Timeout,\n\t\tWriteTimeout: s.Timeout,\n\t}\n\tgo func() {\n\t\t<-ctx.Done()\n\t\tif err := srv.Shutdown(context.Background()); err != nil {\n\t\t\tlog.Printf(\"shutdown error: %v\", err)\n\t\t}\n\t}()\n\tfmt.Fprintf(os.Stdout, \"listening on %s\\n\", s.addr)\n\tif err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {\n\t\treturn fmt.Errorf(\"server error: %w\", err)\n\t}\n\treturn nil\n}\n```"

	return []message.Message{
		{
			ID:    "asst-code-1",
			Role:  message.Assistant,
			Model: models.ModelID("claude-3-5-sonnet-20241022"),
			Parts: []message.ContentPart{
				message.TextContent{Text: "Here is a complete HTTP server implementation:\n\n" + longCode + "\n\nThis implementation provides a thread-safe router with graceful shutdown support."},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
}

func fixtureWithThinking() []message.Message {
	return []message.Message{
		{
			ID:   "user-think-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "What's the complexity of merge sort?"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:    "asst-think-1",
			Role:  message.Assistant,
			Model: models.ModelID("claude-3-5-sonnet-20241022"),
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "The user is asking about merge sort complexity. Let me think through this carefully.\n\nMerge sort works by dividing the array in half, recursively sorting each half, then merging.\n- Division: log n levels\n- Merge at each level: O(n) work\n- Total: O(n log n)\n\nSpace complexity: O(n) auxiliary space for the merge step."},
				message.TextContent{Text: "Merge sort has **O(n log n)** time complexity in all cases (best, average, and worst). It requires **O(n)** auxiliary space for the merging step."},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

// buildToolResults extracts tool results from tool-role messages into a map
// keyed by tool call ID, as expected by block renderers.
func buildToolResults(msgs []message.Message) map[string]message.Message {
	m := make(map[string]message.Message)
	for _, msg := range msgs {
		if msg.Role != message.Tool {
			continue
		}
		for _, part := range msg.Parts {
			if tr, ok := part.(message.ToolResult); ok {
				m[tr.ToolCallID] = msg
			}
		}
	}
	return m
}

