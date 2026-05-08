---
name: "figure_routing"
description: "科研制图路由决策：根据图表类型自动选择最佳生成方法（D2/Mermaid/matplotlib/TikZ/ImageGen）"
category: "research"
tags: ["figure", "visualization", "routing", "diagram"]
version: 1
author: "system"
exposure: implicit
---

# 科研制图路由决策

生成论文插图时，根据图表内容类型选择最佳生成方法。核心原则：**精确内容用代码渲染，概念内容用 AI 生成**。

## 决策路由表

| 图表类型 | 推荐方法 | 工具 | 原因 |
|----------|----------|------|------|
| 架构图/系统组件图 | D2 语法 | DiagramGen | 精确文字标注，矢量渲染 |
| 流程图/决策树 | Mermaid 或 D2 | Bash (mmdc) 或 DiagramGen | Mermaid token 效率最高 |
| 序列图/时序图 | Mermaid | Bash (mmdc) | Mermaid 序列图语法最成熟 |
| 数据可视化（折线/柱状/散点/热力图） | matplotlib + SciencePlots | Bash (python3) | 数据精确，期刊级样式 |
| 数学示意图/几何图 | TikZ | LaTeX 编译 | 精确公式与几何 |
| 概念场景图/抽象示意 | AI 图像生成 | ImageGen (`text_policy=forbid/auto`) | 仅此类适用 AI 生图 |
| 有少量短词的视觉海报/图形摘要 | AI 图像生成（谨慎） | ImageGen (`text_policy=short_text`) | 只允许 1-4 个短词，生成后必须人工检查 |
| 需要后期精确标注的概念底图 | AI 底图 + overlay | ImageGen (`text_policy=overlay_base`) + TikZ/LaTeX | 底图无文字，精确文字后期叠加 |
| 论文原图引用 | PDF 提取 | View + 截图 | 最可靠的方式 |

## 决策流程

1. **识别图表目的**：这张图要传达什么信息？
2. **填写 figure brief**：
   - `figure_type`
   - `purpose`
   - `text_items`
   - `data_source`
   - `route`
   - `prompt`
   - `overlay_plan`
   - `validation_checks`
3. **判断精确性要求**：
   - 精确文字、公式、坐标轴、legend、数据 → 代码生成（D2/matplotlib/TikZ）
   - 概念底图但后期要精确标注 → ImageGen `overlay_base` + TikZ/LaTeX overlay
   - 无文字视觉氛围/概念 → ImageGen `forbid` 或 `auto`
   - 只有 1-4 个短词且用户明确要求 → ImageGen `short_text`，生成后人工检查
4. **选择具体工具**：按上表匹配
5. **生成并验证**：执行后检查输出文件是否存在且有效

## 关键规则

- **永远不要**用 ImageGen 生成包含精确文字的图表（文字会乱码）
- **永远不要**用 ImageGen 生成数据可视化（数据不精确）
- 坐标轴、legend、公式、编号步骤、论文版式、小字标签都属于精确内容，走 D2/matplotlib/TikZ 或 overlay
- ImageGen 默认必须避免文字；短文字必须在 prompt 中明确要求，或显式设置 `text_policy=short_text`
- 如果首选方法失败，按降级路径重试：D2 → Mermaid → 文字描述
- 每张图生成后必须验证输出文件存在
