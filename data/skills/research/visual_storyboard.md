---
name: "visual_storyboard"
description: "在绘图前规划论文图组叙事：把中心贡献拆成 figure sequence、证据需求和生成路线。"
category: "research"
tags: ["figure-planning", "storyboard", "paper-figures", "visual-narrative", "图组规划", "规划论文图组"]
version: 1
author: "system"
when_to_use: "当用户要规划整篇论文的图组结构与叙事顺序时使用，不用于逐像素审稿或直接出图。"
allowed-tools: ["KBQuery", "KBSearch", "View", "Grep", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 论文图组 Storyboard

## 输入与澄清
- 输入：中心贡献、论文草稿结构、已有结果图/表、目标 venue。
- 澄清：页数预算、审稿人关注点、是否已有必保留图。

## 方法论流程
1. Central Contribution：压缩成一句话主张。
2. Reader Journey：定义 Fig1..N 的信息流（问题 -> 方法 -> 结果 -> 分析 -> 局限）。
3. Per-Figure Brief：`claim | evidence | source data | visual form | caption thesis | risk`。
4. Redundancy Audit：识别重复图、无 claim 图、无图支撑 claim。
5. Route Hand-off：按图类型路由到 `figure_routing` 推荐的生成技能。

## 输出模板
- Figure Set Map。
- Per-Figure Brief Cards。
- Missing Evidence Checklist。
- Generation Route Plan。

## 质量门槛
- 每张图必须服务单一 claim。
- 没有 source data 的图只能标概念图或待补证据。
- 先完成 storyboard 再进入生成，避免“先画后想”。
- 不虚构实验结果来填图位。
