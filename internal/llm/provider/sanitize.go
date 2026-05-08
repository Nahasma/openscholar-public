package provider

import "regexp"

// providerTagRe 匹配已知 Provider 注入的 XML 标签
var providerTagRe = regexp.MustCompile(
	`</?(?:minimax|deepseek|zhipu|moonshot|baichuan):[a-zA-Z_]+[^>]*>`,
)

// orphanToolTagRe 匹配孤立的工具调用闭合标签
var orphanToolTagRe = regexp.MustCompile(
	`</(?:tool_call|function_call|tool_use)>`,
)

// invokeBlockRe 匹配 Provider 在文本中输出的整块工具调用 XML（如 MiniMax 的 <invoke>...</invoke>）
var invokeBlockRe = regexp.MustCompile(`(?s)<invoke\s+name="[^"]*">.*?</invoke>`)

// SanitizeContentDelta 清洗 Provider 文本流中的非内容标签。
func SanitizeContentDelta(content string) string {
	content = invokeBlockRe.ReplaceAllString(content, "")
	content = providerTagRe.ReplaceAllString(content, "")
	content = orphanToolTagRe.ReplaceAllString(content, "")
	return content
}
