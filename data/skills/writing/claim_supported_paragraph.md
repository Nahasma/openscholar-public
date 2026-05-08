---
name: "claim_supported_paragraph"
description: "撰写有证据支撑的论文段落：先定义段落功能，再完成 claim-evidence-warrant-limitation 结构。"
category: "writing"
tags: ["paragraph", "claim-evidence", "warrant", "academic-writing", "段落撰写", "有证据支撑的段落"]
version: 1
author: "system"
when_to_use: "当用户需要写或改一个具体论文段落，并要求每个主张有证据支撑时使用，不用于整篇大纲规划。"
allowed-tools: ["KBSearch", "KBQuery", "View", "Grep", "AskUser"]
agent: "research"
effort: "medium"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 证据支撑段落撰写

## 输入与澄清
- 输入：段落所在章节、目标 claim、可用证据/引用、语气要求。
- 澄清：段落功能（motivation/background/method/result/limitation/transition）。

## 方法论流程
1. Paragraph Brief：读者、功能、目标 claim、长度约束。
2. Evidence Map：列证据来源与可用表述强度。
3. Draft CEWL 结构：
- Claim（主题句）
- Evidence（事实或引用）
- Warrant（解释为何支持 claim）
- Limitation/Transition（边界或过渡）
4. Overclaim Audit：降级无证据强断言与夸张词。
5. Revision Pass：给 formal/concise 两种可选版本。

## 输出模板
- Paragraph Brief。
- Evidence Map。
- Draft Paragraph。
- Revised Paragraph Variants。
- Unsupported Claims and Fixes。

## 质量门槛
- 不新增未提供 citation 或实验数值。
- 推测性结论必须显式降级（may/suggests/可能）。
- 证据缺口必须单列，不可用修辞掩盖。
- 最终段落要可追溯到输入证据。
