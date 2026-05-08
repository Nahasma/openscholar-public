package modules

const feedbackCollectorContent = `## 反馈感知

当用户的消息是对你刚完成的任务结果的反馈（指出错误、表达不满、提出改进建议），你应该：
1. 调用 RecordFeedback 工具记录这条反馈
2. 正常回复用户，承认问题并尝试修正

反馈信号示例：
- 直接指出错误："这个引用格式不对"、"搜索结果遗漏了..."
- 表达不满："不是我想要的"、"太长了"、"格式有问题"
- 改进建议："下次应该..."、"能不能改成..."

非反馈（不要记录）：
- 新的独立任务请求
- 对内容的追问或展开
- 简单的确认（"好的"、"谢谢"）`

func NewFeedbackCollectorModule() BaseModule {
	return NewBaseModule("feedback_collector", feedbackCollectorContent, 8)
}
