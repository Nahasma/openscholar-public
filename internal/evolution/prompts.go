package evolution

const analysisPrompt = `你是一个 Agent 技能进化设计师。分析以下用户反馈案例，识别 Agent 的失败模式和根因。

## 当前技能库
%s

%s

## 用户反馈案例
%s

## 任务
分析失败模式，将根因分类为：
- storage_failure: 重要知识/规则未被任何技能覆盖
- retrieval_failure: 技能已存在但未被检索到（描述或标签不匹配）
- quality_failure: 技能已存在且被使用，但指令不够精确/完整

输出 JSON（不要包含代码块标记）:
{
  "failure_patterns": [{"pattern_name": "...", "affected_cases": ["case_id"], "root_cause": "storage_failure | retrieval_failure | quality_failure", "explanation": "...", "potential_fix": "..."}],
  "recommendations": [{"action": "add_new | refine_existing | no_change", "target_skill": "skill_id or new_name", "rationale": "...", "priority": 1}],
  "summary": "..."
}`

const reflectionPrompt = `审视以下分析，检查是否存在误判或遗漏：

## 上一轮分析
%s

## 原始反馈案例
%s

## 当前技能库
%s

请检查：
1. root_cause 分类是否准确？是否有误归因？
2. 是否遗漏了某些反馈案例中的失败模式？
3. 修复建议是否足够具体、可操作？
4. 是否有更简单的修复方案被忽略？

输出改进后的分析 JSON（同格式，不要包含代码块标记）。`

const refinementPrompt = `基于分析结果，对技能库做出最小必要的修改。

## 最终分析
%s

## 完整技能库
%s

%s

## 约束
- 最多 1 项变更（保守策略，防止级联错误）
- 仅可 add_new 或 refine_existing（不可删除）
- instruction 必须包含 Purpose / When to Use / How to Apply / Constraints 四节
- 避免重复历史失败尝试的相同修改方向

输出 JSON（不要包含代码块标记）:
{
  "action": "apply_changes | no_change_needed",
  "summary": "一句话概括",
  "changes": [{
    "action": "add_new | refine_existing",
    "add_new": {"name": "...", "category": "...", "description": "...", "tags": ["..."], "instruction": "完整 Markdown 正文"},
    "refine_existing": {"skill_id": "...", "changes": {"description": "新描述", "instruction": "新指令全文"}},
    "reasoning": "为什么这个变更能解决问题"
  }]
}`
