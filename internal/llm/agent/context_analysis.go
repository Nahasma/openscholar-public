package agent

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/prompt"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// ContextCategory identifies the type of context content.
type ContextCategory string

const (
	ContextUserInput     ContextCategory = "user_input"
	ContextAssistantText ContextCategory = "assistant_text"
	ContextReasoning     ContextCategory = "reasoning"
	ContextToolCalls     ContextCategory = "tool_calls"
	ContextToolResults   ContextCategory = "tool_results"
	ContextMemory        ContextCategory = "memory"
	ContextSessionMemory ContextCategory = "session_memory"
	ContextSystemStatic  ContextCategory = "system_static"
	ContextSystemDynamic ContextCategory = "system_dynamic"
)

// ContextSlice holds token statistics for a single category.
type ContextSlice struct {
	Category ContextCategory
	Tokens   int
	Bytes    int
	Count    int
}

// ToolTokenStats holds aggregated token usage for one tool.
type ToolTokenStats struct {
	CallCount    int
	InputTokens  int
	ResultTokens int
}

// ContextAnalysis is the full context breakdown for a session turn.
type ContextAnalysis struct {
	SessionID          string
	ActualInputTokens  int // API-reported precise total
	EstimatedTokens    int // local estimate (chars/4)
	CacheReadTokens    int
	CacheCreateTokens  int
	ByCategory         []ContextSlice
	ByTool             map[string]ToolTokenStats
	CompactToolResults int
	DataRefs           []string
	ArtifactPaths      []string
	GeneratedAt        time.Time
}

// ContextAnalyzer analyzes the context composition of a session.
type ContextAnalyzer interface {
	Analyze(sessionID string, systemBlocks []prompt.PromptBlock,
		msgs []message.Message, actualInputTokens int) ContextAnalysis
}

type contextAnalyzer struct{}

// NewContextAnalyzer returns a new ContextAnalyzer.
func NewContextAnalyzer() ContextAnalyzer {
	return &contextAnalyzer{}
}

// estimateTokens returns a rough token estimate: chars / 4.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

func (a *contextAnalyzer) Analyze(
	sessionID string,
	systemBlocks []prompt.PromptBlock,
	msgs []message.Message,
	actualInputTokens int,
) ContextAnalysis {
	// accumulate per-category raw bytes
	catBytes := make(map[ContextCategory]int)
	catCount := make(map[ContextCategory]int)
	byTool := make(map[string]ToolTokenStats)
	compactToolResults := 0
	var dataRefs []string
	var artifactPaths []string

	// --- system blocks ---
	for _, blk := range systemBlocks {
		n := len(blk.Text)
		if blk.IsDynamic {
			catBytes[ContextSystemDynamic] += n
			catCount[ContextSystemDynamic]++
		} else {
			catBytes[ContextSystemStatic] += n
			catCount[ContextSystemStatic]++
		}
	}

	// --- conversation messages ---
	for _, msg := range msgs {
		switch msg.Role {
		case message.User:
			// Inspect TextContent to detect memory tags
			var hasSess, hasMem bool
			var textBytes int
			for _, part := range msg.Parts {
				if tc, ok := part.(message.TextContent); ok {
					if strings.Contains(tc.Text, "<session-memory>") {
						hasSess = true
					}
					if strings.Contains(tc.Text, "<memory-context>") {
						hasMem = true
					}
					textBytes += len(tc.Text)
				}
			}
			switch {
			case hasSess:
				catBytes[ContextSessionMemory] += textBytes
				catCount[ContextSessionMemory]++
			case hasMem:
				catBytes[ContextMemory] += textBytes
				catCount[ContextMemory]++
			default:
				catBytes[ContextUserInput] += textBytes
				catCount[ContextUserInput]++
			}

		case message.Assistant:
			for _, part := range msg.Parts {
				switch p := part.(type) {
				case message.TextContent:
					n := len(p.Text)
					catBytes[ContextAssistantText] += n
					catCount[ContextAssistantText]++
				case message.ReasoningContent:
					n := len(p.Thinking)
					catBytes[ContextReasoning] += n
					catCount[ContextReasoning]++
				case message.ToolCall:
					n := len(p.Name) + len(p.Input)
					catBytes[ContextToolCalls] += n
					catCount[ContextToolCalls]++
					// aggregate per-tool call tokens
					st := byTool[p.Name]
					st.CallCount++
					st.InputTokens += estimateTokens(p.Input)
					byTool[p.Name] = st
				}
			}

		case message.Tool:
			for _, part := range msg.Parts {
				if tr, ok := part.(message.ToolResult); ok {
					n := len(tr.Content)
					catBytes[ContextToolResults] += n
					catCount[ContextToolResults]++
					if obs, ok := parseCompactObservation(tr.Content); ok {
						compactToolResults++
						if obs.DataRef != "" {
							dataRefs = appendUniqueString(dataRefs, obs.DataRef)
						}
					}
					if md := parseToolMetadata(tr.Metadata); md != nil {
						for _, path := range contextArtifactPaths(md) {
							artifactPaths = appendUniqueString(artifactPaths, path)
						}
						if dataRef, _ := md["data_ref"].(string); dataRef != "" {
							dataRefs = appendUniqueString(dataRefs, dataRef)
						}
					}
					// aggregate per-tool result tokens
					st := byTool[tr.Name]
					st.ResultTokens += estimateTokens(tr.Content)
					byTool[tr.Name] = st
				}
			}
		}
	}

	// compute estimated total
	totalBytes := 0
	for _, b := range catBytes {
		totalBytes += b
	}
	estimatedTotal := estimateTokens(strings.Repeat("x", totalBytes))
	// simpler: sum per-category estimates
	estimatedTotal = 0
	for _, b := range catBytes {
		estimatedTotal += (b + 3) / 4
	}

	// compute scale factor if actual tokens provided
	scale := 1.0
	if actualInputTokens > 0 && estimatedTotal > 0 {
		scale = float64(actualInputTokens) / float64(estimatedTotal)
	}

	// build ordered ByCategory slice
	categoryOrder := []ContextCategory{
		ContextSystemStatic,
		ContextSystemDynamic,
		ContextSessionMemory,
		ContextMemory,
		ContextUserInput,
		ContextAssistantText,
		ContextReasoning,
		ContextToolCalls,
		ContextToolResults,
	}

	byCategory := make([]ContextSlice, 0, len(categoryOrder))
	for _, cat := range categoryOrder {
		b := catBytes[cat]
		if b == 0 && catCount[cat] == 0 {
			continue
		}
		est := (b + 3) / 4
		scaled := int(float64(est) * scale)
		byCategory = append(byCategory, ContextSlice{
			Category: cat,
			Tokens:   scaled,
			Bytes:    b,
			Count:    catCount[cat],
		})
	}

	result := ContextAnalysis{
		SessionID:          sessionID,
		ActualInputTokens:  actualInputTokens,
		EstimatedTokens:    estimatedTotal,
		ByCategory:         byCategory,
		ByTool:             byTool,
		CompactToolResults: compactToolResults,
		DataRefs:           dataRefs,
		ArtifactPaths:      artifactPaths,
		GeneratedAt:        time.Now(),
	}

	return result
}

type compactObservationInfo struct {
	Tool          string `json:"tool"`
	Status        string `json:"status"`
	PublicSummary string `json:"public_summary"`
	DataRef       string `json:"data_ref"`
}

func parseCompactObservation(raw string) (compactObservationInfo, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return compactObservationInfo{}, false
	}
	var obs compactObservationInfo
	if err := json.Unmarshal([]byte(raw), &obs); err != nil {
		return compactObservationInfo{}, false
	}
	return obs, obs.Tool != "" && obs.Status != "" && obs.PublicSummary != ""
}

func parseToolMetadata(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		return nil
	}
	return md
}

func contextArtifactPaths(md map[string]any) []string {
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			out = appendUniqueString(out, v)
		}
	}
	for _, key := range []string{"path", "svg_path", "png_path", "d2_path"} {
		if v, _ := md[key].(string); v != "" {
			add(v)
		}
	}
	if artifact, ok := md["artifact"].(map[string]any); ok {
		if v, _ := artifact["path"].(string); v != "" {
			add(v)
		}
	}
	if artifacts, ok := md["artifacts"].([]any); ok {
		for _, item := range artifacts {
			if artifact, ok := item.(map[string]any); ok {
				if v, _ := artifact["path"].(string); v != "" {
					add(v)
				}
			}
		}
	}
	if paths, ok := md["artifact_paths"].([]any); ok {
		for _, item := range paths {
			if v, ok := item.(string); ok {
				add(v)
			}
		}
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
