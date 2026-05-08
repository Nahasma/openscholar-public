---
name: "literature_gap_mapping"
description: "围绕研究问题建立可审计检索日志、路线图与 gap 证据卡，输出可验证的研究空白假设。"
category: "research"
tags: ["literature-review", "gap-analysis", "search-protocol", "snowballing", "screening", "综述", "文献综述", "研究空白"]
version: 1
author: "system"
when_to_use: "当用户要做多篇文献综述、相关工作地图或研究空白论证（而非单篇精读）时使用。"
allowed-tools: ["ScholarSearch", "KBList", "KBSearch", "KBQuery", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 文献地图与研究空白分析

## 输入与范围澄清
- 明确 topic、目标读者（导师/审稿人/组会）、时间窗、场景（开题/related work/选题）。
- 设 inclusion/exclusion 标准：任务、数据域、方法类型、年份、语言、可得性。
- 若范围过大，先拆子主题并分别检索。

## 方法论流程
1. Scope & Protocol：定义问题、检索源、关键词组、布尔策略、年份窗口。
2. Search Log：记录 `date | source | query | year window | result count | selected | exclusion reason`。
3. Screening：标题摘要初筛 -> 全文复筛，保留可追溯剔除理由。
4. Snowballing：反向参考文献、前向引用追踪、benchmark/dataset/method 关键词扩展。
5. Route Taxonomy：按 task/method/data/metric/setting/theory/reproducibility 分组。
6. Gap Evidence Card：输出 gap、已有覆盖、缺失证据、反例、可证伪条件、置信度、下一步验证。

## 输出模板
- Scope and Search Protocol。
- Search Log Table。
- Included/Excluded Papers with Reasons。
- Route Map and Debate/Consensus Table。
- Gap Evidence Cards：`gap | coverage | missing evidence | counterexample | falsifier | confidence`。
- Next Search Plan：下一轮查询词与采样策略。

## 质量门槛
- 没有 search log 只能叫 scoping notes，不能叫系统综述。
- 每个 gap 必须给 confidence 与反例检索计划。
- 证据不足时降级为 hypothesis，不输出确定性“空白”。
- 未获取全文的论文仅标 candidate，不做强比较结论。
