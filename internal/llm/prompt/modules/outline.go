package modules

// NewOutlineModule returns the outline expansion workflow module.
func NewOutlineModule() BaseModule {
	return NewBaseModule("outline", outlinePrompt, 30)
}

const outlinePrompt = `# Outline expansion workflow

## 全文写作规划（强制）

当用户请求写完整论文或 survey 时，**必须先规划再写作**，不允许一次性全量生成：

### 1. 规划阶段
- 使用 AskUser 或 plan mode 与用户确认以下内容：
  - 论文大纲（section 结构 + 每 section 关键论点）
  - 每 section 预计引用的关键文献
  - 图表规划：需要哪些图、每张图的类型（TikZ/pgfplots/glm-image）
  - 预计总页数和各 section 篇幅

### 2. Survey 论文额外规划
- 明确与已有 survey 的差异化定位（为什么需要新的 survey？）
- 提出独特的分类框架或组织视角
- 搜索 Semantic Scholar 确认最新文献覆盖

### 3. 分 section 写作
- 大纲确认后，分 section 逐个写作
- 每完成一个 section 编译检查
- 大型任务（>3 sections）应考虑使用 Task 工具并行写作

### 4. 禁止行为
- 禁止不经规划直接生成全文
- 禁止在单次响应中生成超过 2 个 section 的内容

---

## Section 展开策略

When the user provides a paper outline or asks to "expand" a section, use this 4-pass strategy:

## Pass 1: Structure
- Confirm section breakdown and logical flow: Abstract → Introduction → Related Work → Method → Experiments → Conclusion
- Define 2-3 core arguments per section (bullet points)
- Output: section skeleton + key points per section

## Pass 2: Content
- Expand each argument into 1-3 full paragraphs
- Each paragraph follows: topic sentence → evidence/data → concluding sentence
- Add necessary formulas, definitions, citation placeholders (\cite{TODO})
- Output: complete draft text

## Pass 3: Transitions
- Add inter-paragraph transitions and inter-section bridges
- Ensure logical coherence; eliminate abrupt jumps
- Unify terminology and notation
- Output: polished continuous text

## Pass 4: LaTeX formatting
- Convert text to LaTeX format
- Add \label{}, \ref{}, \cite{} markers
- Insert equation/align environments, figure/table references
- Ensure successful compilation
- Output: compilable .tex file(s)

## Notes
- Each pass can be applied per-section (one Edit per .tex file)
- The user may intervene and modify at any pass
- Explain what was changed in each pass`
