package kb

import (
	"strings"
	"testing"
)

func TestParseTreeSelectionResponse_PureJSON(t *testing.T) {
	raw := `{"thinking":"focus methods","node_list":["0015","0016"]}`

	resp, snippet, err := parseTreeSelectionResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "focus methods" {
		t.Fatalf("unexpected thinking: %q", resp.Thinking)
	}
	if len(resp.NodeList) != 2 || resp.NodeList[0] != "0015" || resp.NodeList[1] != "0016" {
		t.Fatalf("unexpected node_list: %#v", resp.NodeList)
	}
	if snippet != raw {
		t.Fatalf("unexpected snippet: %q", snippet)
	}
}

func TestParseTreeSelectionResponse_FencedJSON(t *testing.T) {
	raw := "```json\n{\"thinking\":\"x\",\"node_list\":[\"0001\"]}\n```"

	resp, snippet, err := parseTreeSelectionResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "x" {
		t.Fatalf("unexpected thinking: %q", resp.Thinking)
	}
	if len(resp.NodeList) != 1 || resp.NodeList[0] != "0001" {
		t.Fatalf("unexpected node_list: %#v", resp.NodeList)
	}
	if !strings.HasPrefix(snippet, "{") {
		t.Fatalf("expected object snippet, got: %q", snippet)
	}
}

func TestParseTreeSelectionResponse_WithSurroundingText(t *testing.T) {
	raw := "I will select nodes now.\n{\"thinking\":\"reason\",\"node_list\":[\"0100\"]}\nThese should answer the question."

	resp, snippet, err := parseTreeSelectionResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "reason" {
		t.Fatalf("unexpected thinking: %q", resp.Thinking)
	}
	if len(resp.NodeList) != 1 || resp.NodeList[0] != "0100" {
		t.Fatalf("unexpected node_list: %#v", resp.NodeList)
	}
	if strings.Contains(snippet, "I will select") || strings.Contains(snippet, "These should") {
		t.Fatalf("snippet leaked surrounding text: %q", snippet)
	}
}

func TestParseTreeSelectionResponse_TruncatedJSON(t *testing.T) {
	raw := `{"thinking":"reason","node_list":["0100"]`

	_, snippet, err := parseTreeSelectionResponse(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "truncated JSON object") {
		t.Fatalf("unexpected error: %v", err)
	}
	if snippet == "" {
		t.Fatal("expected bounded snippet for diagnostics")
	}
	if len(snippet) > treeSelectionSnippetLimit+3 {
		t.Fatalf("snippet exceeded bound: %d", len(snippet))
	}
}

func TestParseTreeSelectionResponse_NonObjectArray(t *testing.T) {
	raw := `["0015","0016"]`

	_, snippet, err := parseTreeSelectionResponse(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "must be a JSON object") {
		t.Fatalf("unexpected error: %v", err)
	}
	if snippet == "" {
		t.Fatal("expected snippet")
	}
}

func TestParseTreeSelectionResponse_MissingNodeList(t *testing.T) {
	raw := `{"thinking":"forgot the node list"}`

	_, _, err := parseTreeSelectionResponse(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTreeSelectionResponse_NodeListMustBeArray(t *testing.T) {
	raw := `{"thinking":"bad shape","node_list":"0015"}`

	_, _, err := parseTreeSelectionResponse(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "array of strings") {
		t.Fatalf("unexpected error: %v", err)
	}
}
