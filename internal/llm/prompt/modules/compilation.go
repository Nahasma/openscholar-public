package modules

// NewCompilationModule returns the mandatory compilation verification module.
func NewCompilationModule() BaseModule {
	return NewBaseModule("compilation", compilationPrompt, 62)
}

const compilationPrompt = `
# 编译验证（强制）

## 核心规则
每次写入或修改 .tex 文件后，**必须**立即编译验证。这是强制流程，不可跳过。

## 编译命令
[bash] tectonic main.tex 2>&1

如果 tectonic 不可用：
[bash] pdflatex -interaction=nonstopmode main.tex 2>&1

## 编译前预检（强制，每次编译前执行）

### \bibliographystyle 检查（致命级）
编译前必须执行：
[grep] \\bibliographystyle\{ glob:**/main.tex
如果未找到 → 在 \bibliography{references} 前插入 \bibliographystyle{plainnat}
缺失此声明会导致**全部引用显示为 (?)**

### 硬编码引用检查（致命级）
[grep] \[\d+\] glob:**/sections/*.tex
如果发现正文中的 [数字] 不是 \cite{key} 产生的 → 必须替换为 \cite{key} 命令

## 编译后检查清单

### 1. 错误检查（必须全部为零）
在编译输出中搜索以下关键词：
- "Error" — 编译错误，必须立即修复
- "Citation ... undefined" — 引用 key 不存在，检查 references.bib
- "Reference ... undefined" — \ref 目标不存在，检查 \label
- "File ... not found" — \input 或 \includegraphics 引用的文件缺失
- "I didn't find a database entry" — cite key 在 .bib 中不存在
- 全部引用显示 (?) — 检查是否缺少 \bibliographystyle{}

### 2. 文件完整性检查
写入 .tex 文件后，检查所有 \input{} 引用的文件是否存在：
[grep] \\input\{ glob:**/*.tex
然后用 [glob] 逐一验证文件存在性。

### 3. 警告检查（建议修复）
- "Overfull \\hbox" — 行溢出，调整文本或公式
- "Underfull \\hbox" — 段落排版不佳
- "Missing character" — 字体缺字

### 4. 图表布局检查
- 检查 \begin{figure} 和 \begin{table} 是否集中在文末
- 如果大部分图表远离引用位置，调整为 [htbp] 浮动选项
- 确保每个 section 的配图在该 section 附近而非文末堆积

## 编译修复循环
发现错误 → 定位并修复 → 重新编译 → 检查 → 重复，直到：
- 零编译错误
- 零 undefined reference/citation
- 无 "File not found" 错误

## 最终交付
完成论文写作后，先调用 [PaperValidate] 工具进行全面检查，修复所有 FATAL 和 ERROR 级问题后，再执行最终编译验证，确认 PDF 可正常生成且无关键警告。

## Markdown 项目编译
如果项目使用 Markdown 格式（main.md 而非 main.tex），使用 pandoc 编译：
[bash] pandoc main.md -o paper.pdf --citeproc --bibliography=refs.bib --number-sections 2>&1

如果需要生成 Word 文档：
使用 [DocExport] 工具，或手动执行：
[bash] pandoc main.md -o paper.docx --citeproc --bibliography=refs.bib --number-sections 2>&1`
