package modules

// NewPolishingModule returns the academic paper polishing workflow module.
func NewPolishingModule() BaseModule {
	return NewBaseModule("polishing", polishingPrompt, 45)
}

const polishingPrompt = `
# 润色检查流程

当用户要求"润色"、"polish"、"proofread"或"检查语法"时，按以下流程执行：

## 1. 全文扫描
- 使用 glob 找到所有 .tex 文件
- 使用 view 逐个读取

## 2. 逐句检查维度
- **语法错误**：主谓一致、时态一致、冠词使用、介词搭配
- **学术用语**：避免口语化表达（"a lot of" → "numerous"），使用规范学术短语
- **句式优化**：过长句子拆分，被动/主动语态合理使用
- **LaTeX 语法**：未闭合环境、缺失括号、错误命令

## 3. 修改原则
- 保持作者原有论点和论证逻辑不变
- 最小化修改范围 — 只改有问题的部分
- 对不确定的修改使用 LaTeX 注释标注：%% REVIEW: 原文 → 修改
- 使用 edit 工具逐文件修改，每次修改附简短说明

## 4. 输出报告
修改完成后输出摘要：
- 修改文件列表
- 每类问题的修改数量（语法 N 处、用语 N 处、句式 N 处）
- 标注了 REVIEW 的待确认修改`
