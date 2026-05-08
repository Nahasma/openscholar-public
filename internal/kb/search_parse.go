package kb

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const treeSelectionSnippetLimit = 240

// parseTreeSelectionResponse parses a TreeSearch node-selection response from raw LLM output.
// It accepts a pure JSON object, fenced JSON, or explanatory text containing the first
// balanced JSON object. It returns a bounded snippet for diagnostics.
func parseTreeSelectionResponse(raw string) (treeSearchResponse, string, error) {
	var out treeSearchResponse

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return out, "", errors.New("empty tree selection response")
	}

	jsonObj, extractErr := extractFirstJSONObject(trimmed)
	snippet := boundedSnippet(jsonObj)
	if snippet == "" {
		snippet = boundedSnippet(trimmed)
	}
	if extractErr != nil {
		return out, snippet, fmt.Errorf("failed to extract JSON object from tree selection response: %w", extractErr)
	}

	var shape struct {
		NodeList json.RawMessage `json:"node_list"`
	}
	if err := json.Unmarshal([]byte(jsonObj), &shape); err != nil {
		return out, snippet, fmt.Errorf("failed to parse tree selection JSON object: %w", err)
	}
	if len(shape.NodeList) == 0 {
		return out, snippet, errors.New(`tree selection JSON object missing required "node_list" array`)
	}
	var nodeList []string
	if err := json.Unmarshal(shape.NodeList, &nodeList); err != nil {
		return out, snippet, fmt.Errorf(`tree selection "node_list" must be an array of strings: %w`, err)
	}

	if err := json.Unmarshal([]byte(jsonObj), &out); err != nil {
		return out, snippet, fmt.Errorf("failed to parse tree selection JSON object: %w", err)
	}

	return out, snippet, nil
}

func extractFirstJSONObject(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		if first := firstNonSpaceByte(s); first == '[' {
			return "", errors.New("tree selection response must be a JSON object, got array")
		}
		return "", errors.New("no JSON object found")
	}

	inString := false
	escaped := false
	depth := 0

	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}

	return s[start:], errors.New("truncated JSON object")
}

func firstNonSpaceByte(s string) byte {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return s[i]
		}
	}
	return 0
}

func boundedSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= treeSelectionSnippetLimit {
		return s
	}
	return s[:treeSelectionSnippetLimit] + "..."
}
