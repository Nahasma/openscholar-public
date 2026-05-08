---
name: "paper_argument_outline"
description: "把研究材料组织为可投稿的论证主线：中心贡献、story arc、分节目标与证据链。"
category: "writing"
tags: ["paper-writing", "argument", "story-arc", "outline", "论文结构", "论文", "story", "arc"]
version: 1
author: "system"
when_to_use: "当用户要搭建整篇论文的论证结构与章节框架时使用，不用于单段改写或图表审查。"
allowed-tools: ["KBQuery", "KBSearch", "View", "Grep", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 论文论证大纲

## 输入与澄清
- 输入：研究问题、结果摘要、目标 venue、页数限制、现有草稿。
- 澄清：目标读者、投稿类型（full/short/workshop）、最想强调的贡献。

## 方法论流程
1. One Contribution Rule：先锁定唯一中心贡献。
2. Claim-Data-Logic：为关键 claim 补 evidence 与推理链。
3. Story Arc：problem -> prior limits -> insight -> method -> evidence -> implication。
4. Section Objectives：逐节定义“该节要让读者相信什么”。
5. Missing Evidence Audit：标记论证断点与需补实验/引用。

## 输出模板
- Central Contribution。
- Target Reader and Venue Fit。
- Story Arc Summary。
- Section-by-Section Objective Table。
- Claim-Evidence Chain。
- Missing Evidence Checklist。

## 质量门槛
- 不允许多个互相竞争的中心贡献。
- 每个关键 claim 必须绑定证据或显式标 missing。
- 不凭空生成结果数字或新增引用。
- 大纲应可直接转换为写作任务单。
