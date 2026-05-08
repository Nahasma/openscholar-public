package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/llm/agent/custom"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/message"
)

// generateResult is the JSON structure returned by the LLM.
type generateResult struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	WhenToUse    string   `json:"whenToUse"`
	SystemPrompt string   `json:"systemPrompt"`
	Temperature  float64  `json:"temperature"`
	Tools        []string `json:"tools"`
	Permission   string   `json:"permission"`
}

// Generate uses an LLM to create an AgentConfig from a natural language description.
func Generate(ctx context.Context, description string, p provider.Provider) (*custom.AgentConfig, error) {
	messages := []message.Message{
		{
			Role: "user",
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf(
					"%s\n\n基于以下描述创建一个 Agent 配置：\"%s\"",
					promptGenerate, description,
				)},
			},
		},
	}

	resp, err := p.SendMessages(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM 请求失败: %w", err)
	}

	var result generateResult
	jsonStr := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w\n原始响应: %s", err, resp.Content)
	}

	if result.Name == "" {
		return nil, fmt.Errorf("LLM 生成的 Agent 缺少 name 字段")
	}

	return &custom.AgentConfig{
		Name:         result.Name,
		Description:  result.Description,
		Mode:         "agent",
		WhenToUse:    result.WhenToUse,
		SystemPrompt: result.SystemPrompt,
		Temperature:  result.Temperature,
		Tools:        result.Tools,
		Permission:   result.Permission,
	}, nil
}

// extractJSON extracts a JSON block from an LLM response (first '{' to last '}').
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
