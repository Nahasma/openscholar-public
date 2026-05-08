package modules

// NewAskUserModule returns the AskUser tool usage guidance module.
func NewAskUserModule() BaseModule {
	return NewBaseModule("askuser", askUserPrompt, 15)
}

const askUserPrompt = `
# 模糊指令确认

当用户给出的指令存在歧义或缺少关键信息时，使用 AskUser 工具向用户确认。

## 何时确认
- 指令有 2 种以上合理的理解方式
- 关键参数缺失（如主题范围、目标长度、写作深度）
- 涉及大范围修改（如重构整篇论文结构）

## 何时不确认（直接执行）
- 指令明确、无歧义
- 有上下文的跟进消息（如"继续"、"再修改一下第三段"）
- 简单操作（修改一个段落、修复拼写）
- 用户已在当前对话中阐明过偏好

## 确认原则
- 选项 2-5 个，差异显著
- 始终允许自由输入（allow_freeform: true）
- 确认一次即可，不要反复确认
- 用户跳过时，按最合理理解执行并说明假设
- 每轮对话最多使用 2 次 AskUser
`
