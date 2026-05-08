---
name: "figure_quality_review"
description: "审查科研图表是否清楚、诚实、可发表：检查受众、信息主线、caption、颜色编码与误导风险。"
category: "research"
tags: ["figure", "visualization", "quality-review", "caption", "misleading-risk", "科研绘图", "图表审查", "误导检查", "检查这张图是否误导"]
version: 1
author: "system"
when_to_use: "当用户已有图表并希望审查其表达质量、投稿可用性或误导风险时使用，不负责直接生成新图。"
allowed-tools: ["View", "Glob", "Grep", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 科研图表质量审查

## 输入与澄清
- 输入：图像文件、论文图注草稿、数据说明、目标期刊/会议要求。
- 澄清：目标受众、核心 message、一句话结论、投放介质（单栏/双栏/slide/poster）。

## 方法论流程
1. Figure Brief：audience、message、medium、figure type。
2. Data Integrity：坐标轴截断、比例失真、误差线/样本量/统计说明缺失检查。
3. Visual Encoding：颜色含义、色盲友好、图例一致性、chartjunk 与默认样式风险。
4. Caption Gate：图中内容、来源与方法、读图方式、关键结论、限制。
5. Publication Fit：字体、线宽、panel label、分辨率与版面适配。

## 输出模板
- Figure Brief。
- Major Issues / Minor Issues。
- Misleading Risk Register。
- Caption Rewrite。
- Required Fixes Before Submission。

## 质量门槛
- caption 必填，且包含“图展示什么+如何读+结论+限制”。
- 每个颜色与坐标轴选择都要给理由。
- 只做审查与修改建议；生成路线交给 `figure_routing` 或相应绘图技能。
- 不伪造原始数据、统计结论或显著性说明。
