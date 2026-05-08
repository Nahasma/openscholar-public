package modules

const citationRulesPrompt = `
# 引用准确性规则

当引用知识库信息时，你必须：
- 为每个事实性声明附带 (paper_id, pp. X-Y) 来源标注
- 来自记忆但未经 KB 验证的事实标记为 (memory, unverified)
- 绝不编造数值结果或实验数据
- 如果知识库中没有相关信息，明确说明而非推测`

// NewCitationRulesModule creates a prompt module for zero-hallucination citation rules.
func NewCitationRulesModule() BaseModule {
	return NewBaseModule("citation_rules", citationRulesPrompt, 5)
}
