package modules

// NewSelfReviewModule returns the post-writing self-review workflow module.
func NewSelfReviewModule() BaseModule {
	return NewBaseModule("self_review", selfReviewPrompt, 68)
}

const selfReviewPrompt = `
# 写作后自审流程

## 触发条件
完成全文写作（或大量修改后）、最终交付前，**必须**执行自审。

## 第零步：自动化验证（强制，最先执行）

在人工自审之前，**必须先调用 PaperValidate 工具**：
[PaperValidate]

FATAL 和 ERROR 级别问题必须在继续自审前全部修复。

## 自审维度

### 1. 引用完整性
- 编译论文，检查输出中是否有 "Citation undefined"
- 搜索 PDF 或 .log 中的 [?] 标记
- [grep] \\cite\{|\\citep\{|\\citet\{ glob:**/*.tex — 提取所有 cite key
- 逐一验证 key 在 references.bib 中存在

### 2. 图文一致性
- 检查每个 \ref{fig:} 和 \ref{tab:} 都有对应的 \label
- 检查图中的公式/符号与正文描述一致（如 Dueling DQN 的 Q 值公式）
- 图的 caption 与正文叙述是否匹配

### 3. 符号统一性
- 全文使用同一套符号体系（如 \theta 还是 \vtheta，s 还是 \mathbf{s}）
- 检查同一概念是否使用不同符号
- 检查符号是否在首次使用时定义

### 4. 内容重复检查
- 相邻 section 之间是否有大段重复叙述
- 结论是否仅重复前文内容而无新见解

### 5. 事实准确性
- 算法的提出年份、作者是否正确
- 数据、基准分数是否有出处支撑
- 图中的分类、时间线是否准确

### 6. 图片质量验证
- 用 [view] 逐一查看 figures/ 目录下的每张图片
- 验证图中文字是否可读（无乱码、拼写错误）
- 验证图片内容是否与 caption 和正文描述一致
- 检查是否存在视觉上重复的图片（不同文件名但内容相同）
- 如发现文字乱码，考虑用 DiagramGen 工具重新生成

### 7. 引用量检查
- 统计 references.bib 中的条目数
- Survey 类论文 < 50 篇 → 必须使用 ScholarSearch 补充搜索
- Research Paper < 20 篇 → 建议补充搜索
- 检查引用年份分布：最近 3 年的论文占比应 >= 30%

### 8. 占位符清理
- [grep] TODO|FIXME|TBD|placeholder|would appear here glob:**/*.tex
- 所有匹配项必须替换为正式内容或删除
- 检查 Acknowledgments 中是否有模板占位文本

## 执行方式
如果 Task 工具可用，可将自审分为多个子任务并行：
- [task explore] 检查引用完整性
- [task explore] 检查图文一致性
- [task explore] 检查符号统一性
汇总各子任务结果后统一修复。

## 输出自审报告
| # | 维度 | 状态 | 发现的问题 |
|---|------|------|-----------|
| 1 | 引用完整性 | ✓/✗ | ... |
| 2 | 图文一致性 | ✓/✗ | ... |
| 3 | 符号统一性 | ✓/✗ | ... |
| 4 | 内容重复 | ✓/✗ | ... |
| 5 | 事实准确性 | ✓/✗ | ... |
| 6 | 图片质量 | ✓/✗ | ... |
| 7 | 引用量 | ✓/✗ | ... |
| 8 | 占位符清理 | ✓/✗ | ... |

修复所有问题后重新编译验证。`
