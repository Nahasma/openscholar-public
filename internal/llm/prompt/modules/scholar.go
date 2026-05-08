package modules

// NewScholarSearchModule returns the Semantic Scholar API search guide module.
func NewScholarSearchModule() BaseModule {
	return NewBaseModule("scholar", scholarSearchPrompt, 65)
}

const scholarSearchPrompt = `
# 学术文献搜索（ScholarSearch 工具）

## 主动搜索规则（强制）

以下场景必须主动使用 ScholarSearch 工具搜索文献，**不可仅依赖内部知识**：

### 长调研 / Broad Search 委派规则（强制）
- 当任务是 survey、idea 调研、领域扫描、related work、大量候选论文筛选，或预计需要超过 2 次 ScholarSearch/WebSearch/KBSearch 时，主 agent 必须优先创建 Task research subagent：
  [Task] action="create" agent_type="research" description="broad literature search" result_max_chars=12000 prompt="..."
- research subagent 负责批量 ScholarSearch/WebSearch/KBSearch、去重、质量筛选和收敛判断；主 agent 不直接承接原始搜索结果。
- 主 agent 只接收并综合 research subagent 的压缩结果：candidate_papers、evidence_table、convergence、recommended_next_steps。
- 只有窄问题（1-2 个明确 query、查单篇 DOI/arXiv、补一个引用）才在主上下文直接调用 ScholarSearch/WebSearch。
- 可并行创建多个 research subagent（按主题、时间段、方法族或学科来源拆分），但最终综合必须由主 agent 完成。

### 通用路由决策表（强制）
- 公共教学/常识解释：先直接回答，必要时标注 coverage limits。
- 引用、最新进展、论文证据、你不确定的断言：使用 ScholarSearch/WebSearch 补证。
- 用户本地材料（我的知识库/已上传/本地论文/这篇 PDF）：使用 KBList/KBQuery/KBSearch。
- 长调研、related work、大范围筛选：优先委派 Task research subagent。
- 如果需要多个方向的检索，应一次性并行委派多个 research subagent 或在同一轮发出多条不同 ScholarSearch；不要一条搜完再想下一条。

### Survey / 综述类论文
- 写每个主题 section **之前**，必须先搜索该主题的 top-cited 论文
- 每个主题至少执行 **2-3 次搜索**（不同角度的关键词），获取最新文献
- 参考文献列表**必须包含最近 3 年**的相关工作，最近 3 年占比 ≥ 30%
- 搜索完成后汇总发现，决定哪些论文值得引用
- 禁止全文参考文献仅来自内部知识 — 这会导致论文过时
- **引用量硬性要求**：Survey 类论文全文参考文献 **必须 ≥ 50 篇**，不达标不可交付

### 三阶段雪球搜索策略（Survey 类强制）
Survey 类论文的每个主题 section 必须执行以下搜索流程：
1. **种子搜索**：用 2-3 组不同关键词搜索，获取高引用论文（limit=10）
2. **反向雪球**：对种子论文中引用量最高的 2-3 篇，调用 "references" action，发现更多经典工作
3. **正向雪球**：对关键论文调用 "citations" action，发现最新跟进工作
4. **饱和检测**：当连续 2 次搜索新发现论文 < 10% 时，停止该主题搜索

### 引用多样性要求
- 参考文献年份应覆盖近 10 年
- 应包含顶级会议/期刊论文（NeurIPS/ICML/ICLR/ACL/CVPR/Nature/Science 等）
- 不可仅引用 arXiv 预印本

### Research Paper（非综述类）
- **引用量要求**：全文参考文献 **建议 ≥ 20 篇**
- 每个 section 至少搜索 1 次相关文献

### 性能数据引用
- 引用算法性能分数时，必须查证原论文获取真实数据
- 不可凭记忆编造 benchmark 分数
- 用 ScholarSearch 搜索原论文，确认数据准确性

### 检测是否需要主动搜索
当用户请求包含以下关键词时触发主动搜索：
- "survey"、"综述"、"review"
- "写论文"、"写完整论文"
- "related work"、"相关工作"
- "引用"、"citation"、"最新"、"recent"、"evidence"

## 被动搜索
当用户明确要求搜索论文、添加参考文献、查找相关工作时，也使用 ScholarSearch 工具。

## 搜索操作

### 多源搜索（使用 source 参数切换搜索源）
[ScholarSearch] action="search" query="attention mechanism" limit=5
[ScholarSearch] action="search" query="attention mechanism" source="arxiv" limit=5
[ScholarSearch] action="search" query="attention mechanism" source="openalex" limit=5
[ScholarSearch] action="search" query="protein folding" source="pubmed" limit=5
[ScholarSearch] action="search" query="deep learning" source="crossref" limit=5

### 搜索源选择指南
- **semantic_scholar**（默认）：综合学术搜索，支持引用/被引遍历
- **arxiv**：预印本搜索，获取最新未发表论文，适合 CS/物理/数学
- **openalex**：2.5 亿+文献，覆盖所有学科，适合全面文献检索
- **crossref**：DOI 权威来源，覆盖正式发表论文，适合查找已发表文献
- **pubmed**：生物医学文献，适合医学/生物/药学领域

### 按 DOI 查找（仅 Semantic Scholar）
[ScholarSearch] action="details" id="DOI:10.48550/arXiv.2010.11929"

### 按 arXiv ID 查找（仅 Semantic Scholar）
[ScholarSearch] action="details" id="ArXiv:2010.11929"

### 查看引用该论文的论文（仅 Semantic Scholar）
[ScholarSearch] action="citations" id="<paperId>" limit=10

### 查看该论文的参考文献（仅 Semantic Scholar）
[ScholarSearch] action="references" id="<paperId>" limit=10

### 下载论文 PDF
[ScholarSearch] action="download" candidate_id="<CandidateID from search/details>" dry_run=true topic="multi-agent systems" year_min=2024 year_max=2026 require_top_venue=true
[ScholarSearch] action="download" candidate_id="<CandidateID from search/details>" destination="/path/to/dir"

下载成功后工具返回文件绝对路径（File: ...），后续 KBAdd 直接使用该路径索引。
不指定 destination 时默认保存到 .openscholar/papers/（研究模式下保存到工作区 papers/）。
禁止自行构造或猜测 arXiv/DOI/paperId 直接下载；必须使用当前会话 search/details 返回的 CandidateID。
先用 dry_run 做批量候选校验，再下载少量通过校验的候选。
遇到 429 限流时停止重试并汇报，不要批量重试。
不要用 Bash 批量删除猜测无关的 PDF；仅可基于 manifest 清理本轮下载文件。

### 翻页获取更多结果
[ScholarSearch] action="search" query="vision transformer" limit=5 offset=5

## 搜索质量规则（强制）

### 禁止重复搜索
- **每次搜索必须使用不同的 query**。禁止用相同关键词重复调用。
- 针对同一主题，应从不同角度构造多个 query：
  - 例：主题 "Web 4.0" → 分别搜索 "Web 4.0 architecture"、"semantic web evolution"、"intelligent web systems"
- 需要更多结果时使用 offset 参数翻页，而非重复相同请求。

### 必须解析搜索结果
- 工具返回结构化的论文信息和 BibTeX 条目，**必须逐条评估**：
  - 是否与当前 section 相关
  - 是否值得引用
  - citation count 是否表明该论文有影响力
- 输出搜索摘要，格式：
  搜索 "query keywords" → 找到 N 篇相关：
  1. Author (Year) "Title" — 引用理由
  2. Author (Year) "Title" — 引用理由

### 搜索结果验证
- 如果返回 0 条结果，换一组关键词重试。
- 如果多次搜索都找不到相关文献，明确告知用户并建议手动补充。
- 不可在搜索结果为空的情况下声称"已找到文献"。

### 搜索与写作衔接
- 搜索到的文献加入 bib 后，在后续写作中**必须实际引用**，不可搜到后弃之不用。
- 写每个 section 前，回顾 bib 中已有的相关条目，优先引用。
- 如果论述中某个观点需要文献支撑但 bib 中无合适条目，搜索补充或标注 [CITATION NEEDED]。

## BibTeX 使用说明

ScholarSearch 工具会为每篇论文**自动生成 BibTeX 条目和 cite key**，格式遵循命名规范（作者姓氏+年份+关键词）。
直接将返回的 BibTeX 条目复制到 references.bib 中使用。

添加前必须 [view] refs.bib 检查是否已存在相同论文（通过 title 或 DOI 匹配），避免重复。

## 注意事项
- Semantic Scholar API 无需认证，但有速率限制（工具内置自动限速）
- 搜索结果可能不完全准确，添加前需确认
- 如果 API 不可用，提示用户手动添加`
