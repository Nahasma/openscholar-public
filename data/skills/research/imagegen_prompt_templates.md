---
name: "imagegen_prompt_templates"
description: "AI 图像生成（ImageGen）的结构化 Prompt 模板，仅用于概念场景图"
category: "research"
tags: ["imagegen", "prompt-engineering", "figure", "AI-image"]
version: 2
author: "system"
exposure: implicit
---

# ImageGen 结构化 Prompt 模板

ImageGen（AI 图像生成）**仅适用于概念场景图、抽象示意图和无文字底图**。需要精确文字、公式、坐标轴、legend 或数据的图表应使用 D2/Mermaid/matplotlib/TikZ，或先生成无文字底图再用 LaTeX/TikZ overlay。

## 核心原则

1. **使用英文 prompt**（所有模型英文效果更好）
2. **指定白色背景**：`white background`（适合论文插入）
3. **先定文字策略**：`auto`、`forbid`、`short_text`、`overlay_base`
4. **默认禁止文字**：无明确短文字意图时写 `no text, no labels, no annotations`
5. **明确指定风格**：`scientific illustration`, `clean diagram`, `minimalist`
6. **低杂讯构图**：补充 `low clutter composition`，避免背景过满
7. **描述构图而非细节**：重点在布局和视觉关系
8. **短语优于长句**：保持 prompt 5-7 个描述符，平衡具体性和灵活性

## Figure Brief 固定流程

生成或规划图片前，先写出简短 brief：

- `figure_type`：conceptual_scene / graphical_abstract / architecture / data_plot / formula_geometry
- `purpose`：这张图要支持论文中的哪一句结论
- `text_items`：无 / 1-4 个短词 / 精确标签、公式、坐标轴、legend
- `data_source`：无 / 表格 / 实验结果 / 公式
- `route`：ImageGen / ImageGen + TikZ overlay / DiagramGen(D2) / Mermaid / matplotlib / TikZ
- `prompt`：任务、主体、关系、环境/风格、约束
- `overlay_plan`：需要后期叠加文字时列出坐标和文字
- `validation_checks`：无乱码、事实关系正确、论文背景适配、可读性

## `text_policy` 选择

- `auto`：默认且保守。无文字意图时禁止文字；prompt 明确要求 1-4 个短词时按短文字处理；精确标签/公式/坐标轴时生成 overlay 底图并提示改用代码图。
- `forbid`：强制无文字、无数字、无标注。
- `short_text`：只适合用户明确要求的 1-4 个短词；禁止模型添加额外文字。
- `overlay_base`：生成低杂讯、有留白、无文字底图，供 TikZ/LaTeX 后期叠加。

## Prompt 防乱码技巧（调研结论）

根据 Ideogram 3.0、FLUX.2、DALL-E 3 的最佳实践：

- **最有效方法**：直接禁止文字 — `"Do not include any text, labels, numbers, or annotations"`
- **如必须有短文字**：设置 `text_policy: short_text`，用双引号包裹，且限制在 1-4 个单词内 — `with the word "AI" in bold`
- **如必须有精确文字/公式/坐标轴**：不要直接用 ImageGen 生成文字；用 DiagramGen/matplotlib/TikZ，或 `text_policy: overlay_base` 后再 overlay
- **指定字体风格**：`"bold sans-serif letters"`, `"clean typography"` 可提升文字清晰度
- **模型选择**：CogView-4 中文最佳，DALL-E 3 英文最佳，均不适合长文本渲染

> **系统会自动增强 prompt**（`enhanceImagePrompt`）：按 `text_policy` 追加防乱码或短文字约束、白色背景、低杂讯、学术风格。若你显式要求 `photorealistic` 或 `dark background`，系统不会强行覆盖。

## Prompt 模板

### 概念示意图

```
Task: create a conceptual scientific illustration.
Subject: [概念名称] with [核心元素1] and [核心元素2].
Relationship: show [元素关系/信息流/对比关系].
Environment/style: minimalist, white background, professional color palette.
Constraints: low clutter, no text, no labels, no annotations.
```

### 应用场景图

```
Task: depict an application scenario.
Subject: [场景描述] with [具体元素].
Relationship: emphasize [使用流程/交互关系/前后变化].
Environment/style: clean modern academic illustration, white background.
Constraints: no text, no labels, no annotations.
```

### 对比示意图

```
Task: create a split comparison visual.
Subject: [方法A] on the left and [方法B] on the right.
Relationship: contrast [对比维度] through shapes, color, and layout.
Environment/style: consistent clean scientific style, white background.
Constraints: no text, no labels; use visual separation only.
```

### 抽象流程概念图

```
Task: create an abstract process base image.
Subject: [流程名称] flowing left to right with geometric shapes and connecting lines.
Relationship: color-code stages visually without written labels.
Environment/style: clean scientific style, white background.
Constraints: no text; reserve whitespace if labels will be overlaid later.
```

## 混合方案：AI 底图 + LaTeX 文字叠加

当图片**必须包含文字标注**但适合用 AI 生成底图时：

1. **用 ImageGen 生成无文字底图**：设置 `text_policy: overlay_base`，prompt 中明确 `no text, no labels`
2. **用 TikZ overlay 添加标注**：在 LaTeX 中叠加文字

```latex
\begin{figure}[t]
\centering
\begin{tikzpicture}
  \node[anchor=south west,inner sep=0] (image) at (0,0)
    {\includegraphics[width=0.8\textwidth]{figures/concept.png}};
  \begin{scope}[x={(image.south east)},y={(image.north west)}]
    % 在图片上叠加文字标注（坐标 0-1 范围）
    \node[fill=white, fill opacity=0.8, text opacity=1] at (0.2, 0.8) {Module A};
    \node[fill=white, fill opacity=0.8, text opacity=1] at (0.8, 0.8) {Module B};
    \draw[->, thick] (0.35, 0.8) -- (0.65, 0.8);
  \end{scope}
\end{tikzpicture}
\caption{System architecture with annotated components.}
\label{fig:architecture}
\end{figure}
```

## 不适用场景（禁止使用 ImageGen）

- 包含精确文字标注的技术架构图 -> 用 DiagramGen (D2)
- 数据图表（柱状图、折线图等） -> 用 matplotlib
- 数学公式或几何图形 -> 用 TikZ
- 代码结构或类关系 -> 用 Mermaid

## 质量提醒

AI 生成的图片无法自动验证质量。生成后应：
1. 告知用户 "此图为 AI 生成，建议人工检查准确性"
2. 如果图中出现乱码文字，用 DiagramGen 或 Mermaid 重新生成
3. 自审阶段用 View 工具逐张检查所有 AI 生成图片
4. 生成成功后直接汇报保存路径并继续写作，不要再跑额外文件检查命令
