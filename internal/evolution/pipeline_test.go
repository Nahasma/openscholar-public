package evolution

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractJSON_PureJSON(t *testing.T) {
	raw := `{"action": "no_change_needed", "summary": "ok"}`
	result := extractJSON(raw)
	assert.Contains(t, string(result), `"action"`)
}

func TestExtractJSON_MarkdownCodeBlock(t *testing.T) {
	raw := "```json\n{\"action\": \"apply_changes\"}\n```"
	result := extractJSON(raw)
	assert.Contains(t, string(result), `"action"`)
}

func TestExtractJSON_WithSurroundingText(t *testing.T) {
	raw := "Here is the analysis:\n{\"summary\": \"test\"}\nDone."
	result := extractJSON(raw)
	assert.Contains(t, string(result), `"summary"`)
}

func TestFormatCases(t *testing.T) {
	cases := []EvolutionCase{
		{
			ID:          "12345678-abcd-efgh",
			UserRequest: "写论文",
			Feedback:    "格式不对",
			SkillID:     "writing/format",
		},
		{
			ID:          "87654321-wxyz-ijkl",
			UserRequest: "搜索文献",
			AgentOutput: "found 10 papers",
			Feedback:    "遗漏了关键论文",
		},
	}

	text := formatCases(cases)
	assert.Contains(t, text, "Case 1")
	assert.Contains(t, text, "Case 2")
	assert.Contains(t, text, "写论文")
	assert.Contains(t, text, "格式不对")
	assert.Contains(t, text, "writing/format")
	assert.Contains(t, text, "未识别") // case 2 has no skill_id
}
