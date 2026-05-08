package modules

// NewDocExportModule returns the document export guidance module.
func NewDocExportModule() BaseModule {
	return NewBaseModule("doc_export", docExportPrompt, 63)
}

const docExportPrompt = `
# 文档导出

## 支持的格式
DocExport 工具支持双向转换：LaTeX (.tex) ↔ Word (.docx) ↔ Markdown (.md) ↔ PDF。

## PDF 导出
当用户请求生成 PDF 时，直接调用 DocExport 工具，设置 output_path 为 .pdf 文件：
- 工具内部会自动选择最佳 PDF 引擎（weasyprint/tectonic/xelatex/typst）
- 中文/CJK 内容会自动检测并配置字体，无需手动处理
- 如果导出失败，工具会返回结构化错误信息和建议的下一步操作

### PDF 导出注意事项
- Markdown 通用文档默认通过 HTML/CSS 渲染（weasyprint），排版质量好
- LaTeX 学术项目默认使用原生编译器（tectonic/xelatex），保持模板 fidelity
- 可通过 profile 参数指定："markdown-general"、"latex-project"、"auto"（默认）
- 可通过 language 参数提示语言："zh-CN"、"en"、"auto"（默认自动检测）

### 不要做的事
- 不要手动运行 pandoc 命令——使用 DocExport 工具
- 不要手动处理字体——工具内部自动解决
- 不要在 PDF 失败后反复尝试不同引擎——工具内部已有 fallback 链

## DOCX 导出
调用 DocExport 工具，设置 output_path 为 .docx 文件：
- input_path: LaTeX 或 Markdown 源文件
- reference_doc: 可选的 Word 样式模板
- bibliography: 可选的 .bib 文件（默认自动检测）

### DOCX 已知限制
- 复杂数学公式在 Word 中可能渲染不完美
- TikZ 图形不自动转换，需提前生成 PNG/PDF
- 自定义 LaTeX 命令可能无法解析
- 复杂表格可能需要在 Word 中手动调整

## 导出后验证
1. 确认输出文件已生成
2. 检查 DocExport 返回的 warnings 和 engine_used 信息
3. 对 PDF：如有乱码报告，检查返回的 font_used 信息
`
