---
name: "graphical_abstract"
description: "规划图形摘要的关键信息与视觉元素，输出可执行的草图方案和生成路线。"
category: "research"
tags: ["graphical-abstract", "visual-summary", "pictogram", "layout", "图形摘要"]
version: 1
author: "system"
when_to_use: "当用户要为论文或投稿系统准备图形摘要（graphical abstract）时使用，不替代精确数据图制作。"
allowed-tools: ["View", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 图形摘要规划

## 输入与澄清
- 输入：论文核心贡献、摘要文本、目标期刊要求、已有示意图元素。
- 澄清：读者范围（领域内/跨领域/公众）、版式比例、是否允许文字。

## 方法论流程
1. Key Message：压缩为 1-2 句核心信息。
2. Visual Vocabulary：实体、过程、对比、结果、边界条件。
3. Layout Pattern：left-to-right、before/after、central mechanism、input-model-output。
4. Draft Pipeline：文字草图 -> 粗稿路线 -> 反馈迭代 -> 最终交付路线。
5. Text Policy：先列 `text_items`；无文字用 ImageGen `forbid/auto`，1-4 个短词才可 `short_text`，精确文本、公式、数字、坐标轴优先后期叠加或代码图。

## 输出模板
- Key Message Statement。
- Visual Elements List。
- Layout Plan。
- Prompt / Diagram Route：说明是否为 ImageGen、ImageGen + TikZ overlay、D2、Mermaid、matplotlib 或 TikZ。
- Text Items and Overlay Plan：列出要出现的每个词/公式/数字；精确项必须进入 overlay/code route。
- Feedback Questions。
- Failure Modes and Mitigations。

## 质量门槛
- 不承诺一次生成可发表终稿，必须有迭代步骤。
- 不用 AI 直接生成精确数据、公式排版、坐标轴、legend 或小字密集内容。
- 若使用 ImageGen，必须明确 `text_policy`，并记录无文字/短文字/overlay_base 的理由。
- 图形摘要应服务单一主线信息，避免并列多个核心结论。
- 不虚构机制或结果关系。
