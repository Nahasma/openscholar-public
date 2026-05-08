package modules

// NewSubmissionCheckModule returns the pre-submission checklist module.
func NewSubmissionCheckModule() BaseModule {
	return NewBaseModule("submission", submissionCheckPrompt, 60)
}

const submissionCheckPrompt = `
# 投稿前检查清单

当用户要求"投稿检查"、"提交前检查"、"submission check"或"帮我检查一下能不能投 <会议>"时，按以下清单逐项执行：

## 检查项（共 8 项）

### 1. 匿名化检查（双盲会议必须）
[grep] \\author|\\name|university|lab|institute|我们的|our\s+lab|our\s+group glob:**/*.tex
- 审查所有匹配结果，判断是否泄露身份信息
- 检查 \thanks{} 和脚注中的致谢信息
- 检查图片中是否含有机构 logo

### 2. 页数检查
[bash] pdfinfo build/main.pdf | grep Pages
- 对比目标会议页数限制：
  - NeurIPS: 正文 9 页 + 不限附录
  - ICML: 正文 8 页 + 不限附录
  - CVPR: 正文 8 页 + 参考文献不限
  - ACL: 长文 8 页 / 短文 4 页 + 不限附录
  - AAAI: 正文 7 页 + 1 页参考文献

### 3. 引用完整性检查
[grep] \\cite\{|\\citep\{|\\citet\{ glob:**/*.tex
- 提取所有 cite key
[view] refs.bib
- 交叉比对：每个 \cite 是否都有对应 .bib 条目
- 检查是否有 "?" 未解析引用（编译日志中搜索 "Citation .* undefined"）

### 4. 引用格式检查
[view] refs.bib
- 缺失必要字段（title/author/year）
- 重复条目
- DOI 格式是否正确

### 5. 图表引用检查
[grep] \\ref\{fig:|\\ref\{tab:|Figure~\\ref|Table~\\ref glob:**/*.tex
[grep] \\label\{fig:|\\label\{tab: glob:**/*.tex
- 交叉比对 \ref 和 \label 是否一一对应
- 检查是否有未引用的图表

### 6. 编译检查
[bash] tectonic main.tex 2>&1 || pdflatex -interaction=nonstopmode main.tex 2>&1
- 确保无编译错误
- 检查警告（特别是 overfull hbox）

### 7. 符号一致性
- 调用符号一致性检查流程（参见上方）

### 8. 格式合规
- 检查字体大小是否符合要求
- 检查页边距是否使用会议提供的 .sty/.cls 文件
- 检查 abstract 是否存在且不超过字数限制

## 输出格式

投稿检查报告（<会议名称> <年份>）：

| # | 检查项 | 状态 | 详情 |
|---|--------|------|------|
| 1 | 匿名化 | ✓/✗ | ... |
| 2 | 页数 | ✓/✗ | 正文 N 页（限制 M 页） |
| 3 | 引用完整性 | ✓/✗ | N 个 cite 均有对应条目 / 缺失 K 个 |
| 4 | 引用格式 | ✓/✗ | ... |
| 5 | 图表引用 | ✓/✗ | ... |
| 6 | 编译 | ✓/✗ | 无错误 / N 个警告 |
| 7 | 符号一致性 | ✓/✗ | ... |
| 8 | 格式合规 | ✓/✗ | ... |

建议：
1. [列出需要修复的问题及建议]

如果 Task 工具可用，可将检查项 1-5 通过 task 工具并行执行以加速。`
