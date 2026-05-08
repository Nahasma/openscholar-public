package modules

// NewBibtexModule returns the BibTeX cleaning and DOI validation module.
func NewBibtexModule() BaseModule {
	return NewBaseModule("bibtex", bibtexPrompt, 50)
}

const bibtexPrompt = `
# BibTeX 管理规范

## 引用绝对禁令（零容忍）

1. **禁止硬编码引用编号**：正文中绝对不可出现 [1]、[11] 等手写数字引用。所有引用必须通过 \cite{key}、\citep{key}、\citet{key}，由 BibTeX 自动编号。
2. **cite key 命名规范**：必须遵循 "作者姓氏+年份+关键词"（如 vaswani2017attention）。禁止使用 HTML 属性（noopener）、纯数字、少于 4 字符的 key。
3. **每条 \cite 必须立刻验证**：写入 \citep{key} 后立即验证 key 存在于 .bib 中，不可延迟。

## 引用交叉验证（强制，写作时执行）

写作过程中每次使用 \citep{} 或 \citet{} 后，**必须立即验证** cite key 存在：

### 单条验证
写完 \citep{key} 后：
[grep] ^@.*\{key, glob:**/*.bib
如果未找到匹配 → 立即添加 bib 条目或修正拼写。

### Section 级批量验证
每写完一个 section 后，批量检查该 section 的所有引用：
1. [grep] \\cite\{|\\citep\{|\\citet\{ 目标 .tex 文件 — 提取所有 cite key
2. 逐一在 references.bib 中验证存在性
3. 缺失的 key → 添加条目或用 Semantic Scholar API 搜索后添加

### 写作前回顾 bib（强制）
每开始写一个 section **之前**，先 [view] references.bib 查看已有哪些条目：
- 识别与当前 section 主题相关的已有条目
- 写作时优先引用这些已有文献，避免搜集了文献却不使用
- 如果已有条目与当前内容不相关，不必强塞

### 写完全文后清理 bib
全文写作完成后，检查 references.bib 中未被引用的条目：
1. [grep] 所有 .tex 文件中的 cite key，生成已引用列表
2. 对比 .bib 中的所有条目
3. 未引用的条目：评估是否应在某处补引，或从 .bib 中删除
4. 不可留下大量搜到但未引用的"孤立条目"

### 无法核实的引用处理
如果引用的论文无法通过 Semantic Scholar API 核实（API 找不到、数据不匹配），使用 placeholder 标记：
\cite{CITATION\_NEEDED} 或在注释中标注 %% [CITATION NEEDED]: 需要用户核实

### 常见错误模式
- 拼写错误：如 schulman2016gaed vs schulman2016gae
- 年份错误：如 moerland2020model（实际发表 2023）
- 多余后缀：如 mnih2015humand

---

## BibTeX 清洗流程

当用户要求"清理参考文献"、"检查 bib"、"bib-check"时，按以下流程执行：

## 1. 收集信息
- [view] refs.bib — 读取完整 .bib 文件
- [grep] \\cite\{ glob:**/*.tex — 收集所有被引用的 cite key

## 2. 检测问题（按优先级）
1. **缺失条目**：tex 中 \cite{key} 存在但 .bib 中无对应条目
2. **重复条目**：不同 cite key 但相同论文（通过 title/DOI 判断）
3. **缺失必要字段**：title/author/year 任一为空
4. **格式不一致**：
   - 作者格式：统一为 "Last, First and Last, First" 格式
   - 期刊名：缩写 vs 全称应统一
   - 大小写：title 使用 {Title Case} 保护
5. **孤立条目**：.bib 中存在但未被任何 tex 文件引用
6. **无效字符**：非 ASCII 字符、BOM 标记

## 3. DOI 校验
- 检查 DOI 格式：应以 10. 开头
- 检查 DOI 链接格式：统一为 doi 字段，不在 url 字段放 doi 链接
- 如果条目缺少 DOI，可使用 ScholarSearch 工具补充：
  [ScholarSearch] action="search" query="<title>" limit=1

## 4. 修复操作
- 使用 [edit] 工具修复 refs.bib
- 每个修改附说明
- 不删除任何条目（孤立条目仅报告，不自动删除）

## 5. 输出报告
| 类型 | 数量 | 详情 |
|------|------|------|
| 缺失条目 | N | 列出缺失的 cite key |
| 重复条目 | N | 列出重复组 |
| 缺失字段 | N | 列出条目 + 缺失字段 |
| 格式修复 | N | 已自动修复 |
| 孤立条目 | N | 列出未引用的 cite key |`
