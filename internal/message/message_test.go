package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage_Content(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			TextContent{Text: "hello world"},
		},
	}
	assert.Equal(t, "hello world", msg.Content().Text)
}

func TestMessage_ContentEmpty(t *testing.T) {
	msg := Message{Parts: []ContentPart{}}
	assert.Equal(t, "", msg.Content().Text)
}

func TestMessage_ToolCalls(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			TextContent{Text: "thinking..."},
			ToolCall{ID: "tc1", Name: "read_file", Input: `{"path": "test.go"}`, Finished: true},
			ToolCall{ID: "tc2", Name: "write_file", Input: `{"path": "out.go"}`, Finished: true},
		},
	}
	tcs := msg.ToolCalls()
	assert.Len(t, tcs, 2)
	assert.Equal(t, "read_file", tcs[0].Name)
	assert.Equal(t, "write_file", tcs[1].Name)
}

func TestMessage_ToolResults(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			ToolResult{ToolCallID: "tc1", Name: "read_file", Content: "file contents"},
		},
	}
	trs := msg.ToolResults()
	assert.Len(t, trs, 1)
	assert.Equal(t, "tc1", trs[0].ToolCallID)
	assert.Equal(t, "file contents", trs[0].Content)
}

func TestMessage_AppendContent(t *testing.T) {
	msg := Message{}
	msg.AppendContent("hello ")
	msg.AppendContent("world")
	assert.Equal(t, "hello world", msg.Content().Text)
}

func TestMessage_AddToolCall(t *testing.T) {
	msg := Message{}
	msg.AddToolCall(ToolCall{ID: "tc1", Name: "test"})
	assert.Len(t, msg.ToolCalls(), 1)

	// Update same ID
	msg.AddToolCall(ToolCall{ID: "tc1", Name: "test_updated"})
	tcs := msg.ToolCalls()
	assert.Len(t, tcs, 1)
	assert.Equal(t, "test_updated", tcs[0].Name)
}

func TestMessage_IsFinished(t *testing.T) {
	msg := Message{}
	assert.False(t, msg.IsFinished())

	msg.AddFinish(FinishReasonEndTurn)
	assert.True(t, msg.IsFinished())
	assert.Equal(t, FinishReasonEndTurn, msg.FinishReason())
}

func TestMessage_CleanIncompleteToolCalls(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			TextContent{Text: "text"},
			ToolCall{ID: "tc1", Name: "valid", Input: `{"key":"val"}`, Finished: true},
			ToolCall{ID: "", Name: "", Input: "", Finished: false},               // empty input
			ToolCall{ID: "tc3", Name: "bad", Input: "not-json", Finished: false}, // invalid JSON
		},
	}
	msg.CleanIncompleteToolCalls()
	tcs := msg.ToolCalls()
	assert.Len(t, tcs, 1)
	assert.Equal(t, "valid", tcs[0].Name)
}

func TestNormalizeToolCallInput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{
			name:   "valid JSON object",
			input:  `{"file_path":"/tmp/test.go"}`,
			want:   `{"file_path":"/tmp/test.go"}`,
			wantOK: true,
		},
		{
			name:   "double-encoded JSON string",
			input:  `"{\"file_path\":\"/tmp/test.go\"}"`,
			want:   `{"file_path":"/tmp/test.go"}`,
			wantOK: true,
		},
		{
			name:   "empty string",
			input:  "",
			want:   "",
			wantOK: false,
		},
		{
			name:   "garbage input",
			input:  `not-json-at-all`,
			want:   "",
			wantOK: false,
		},
		{
			name:   "JSON string (not object)",
			input:  `"just a string"`,
			want:   "",
			wantOK: false,
		},
		{
			name:   "JSON array (not object)",
			input:  `[1,2,3]`,
			want:   "",
			wantOK: false,
		},
		{
			name:   "empty JSON object",
			input:  `{}`,
			want:   `{}`,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeToolCallInput(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.JSONEq(t, tt.want, got)
			}
		})
	}
}

func TestSetToolCalls_DropsInvalidInput(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			TextContent{Text: "text"},
		},
	}
	msg.SetToolCalls([]ToolCall{
		{ID: "tc1", Name: "valid", Input: `{"key":"val"}`},
		{ID: "tc2", Name: "bad", Input: `not-json`},
		{ID: "tc3", Name: "double-encoded", Input: `"{\"k\":\"v\"}"`},
	})
	tcs := msg.ToolCalls()
	assert.Len(t, tcs, 2)
	assert.Equal(t, "tc1", tcs[0].ID)
	assert.Equal(t, "tc3", tcs[1].ID)
}

func TestMarshallUnmarshallParts(t *testing.T) {
	parts := []ContentPart{
		TextContent{Text: "hello"},
		ToolCall{ID: "tc1", Name: "test", Input: `{"key":"val"}`, Type: "function", Finished: true},
		ToolResult{ToolCallID: "tc1", Name: "test", Content: "result"},
		Finish{Reason: FinishReasonEndTurn},
	}

	data, err := marshallParts(parts)
	require.NoError(t, err)

	restored, err := unmarshallParts(data)
	require.NoError(t, err)
	require.Len(t, restored, 4)

	// Check text
	tc, ok := restored[0].(TextContent)
	require.True(t, ok)
	assert.Equal(t, "hello", tc.Text)

	// Check tool call
	toolCall, ok := restored[1].(ToolCall)
	require.True(t, ok)
	assert.Equal(t, "tc1", toolCall.ID)
	assert.Equal(t, "test", toolCall.Name)

	// Check tool result
	toolResult, ok := restored[2].(ToolResult)
	require.True(t, ok)
	assert.Equal(t, "tc1", toolResult.ToolCallID)

	// Check finish
	finish, ok := restored[3].(Finish)
	require.True(t, ok)
	assert.Equal(t, FinishReasonEndTurn, finish.Reason)
}

func TestMarshallParts_ReasoningContent(t *testing.T) {
	parts := []ContentPart{
		ReasoningContent{Thinking: "let me think..."},
	}
	data, err := marshallParts(parts)
	require.NoError(t, err)

	restored, err := unmarshallParts(data)
	require.NoError(t, err)
	require.Len(t, restored, 1)

	rc, ok := restored[0].(ReasoningContent)
	require.True(t, ok)
	assert.Equal(t, "let me think...", rc.Thinking)
}

func TestFinishReasonConstants(t *testing.T) {
	assert.Equal(t, FinishReason("end_turn"), FinishReasonEndTurn)
	assert.Equal(t, FinishReason("max_tokens"), FinishReasonMaxTokens)
	assert.Equal(t, FinishReason("tool_use"), FinishReasonToolUse)
}

func TestMessage_MarkToolCallInputDoneDoesNotFinishExecution(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			ToolCall{
				ID:       "tc1",
				Name:     "Read",
				Input:    `{"file":"main.go"}`,
				State:    ToolCallQueued,
				Finished: false,
			},
		},
	}

	msg.MarkToolCallInputDone("tc1")
	tcs := msg.ToolCalls()
	require.Len(t, tcs, 1)
	assert.False(t, tcs[0].Finished)
	assert.True(t, tcs[0].InputDone)
	assert.Equal(t, ToolCallQueued, tcs[0].EffectiveState())
}

func TestMessage_InitToolCallsQueued_AssignsBatchIDs(t *testing.T) {
	msg := Message{
		Parts: []ContentPart{
			ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"query":"a"}`},
			ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"query":"b"}`},
			ToolCall{ID: "tc3", Name: "WebSearch", Input: `{"query":"c"}`},
			ToolCall{ID: "tc4", Name: "WebSearch", Input: `{"query":"d"}`},
			ToolCall{ID: "tc5", Name: "ScholarSearch", Input: `{"query":"e"}`},
		},
	}

	msg.InitToolCallsQueued()
	tcs := msg.ToolCalls()
	require.Len(t, tcs, 5)

	assert.Equal(t, "ScholarSearch#1", tcs[0].BatchID)
	assert.Equal(t, "ScholarSearch#1", tcs[1].BatchID)
	assert.Equal(t, "WebSearch#2", tcs[2].BatchID)
	assert.Equal(t, "WebSearch#2", tcs[3].BatchID)
	assert.Equal(t, "ScholarSearch#3", tcs[4].BatchID)
	assert.Equal(t, ToolCallQueued, tcs[0].State)
	assert.Equal(t, 0, tcs[0].Order)
	assert.Equal(t, 4, tcs[4].Order)
}
