---
name: "reproducibility_triage"
description: "对实验结果执行可复现分诊：追踪来源、参数、环境与手工步骤，给出修复优先级。"
category: "workflow"
tags: ["reproducibility", "experiment", "provenance", "seed", "可复现", "复现实验", "参数"]
version: 1
author: "system"
when_to_use: "当用户要判断某个图表、指标或实验结论是否可复现，并定位缺失元数据时使用。"
allowed-tools: ["Glob", "Grep", "View", "KBSearch", "KBQuery", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 可复现分诊

## 输入与澄清
- 输入：目标结果标识（figure/table/metric）、代码与日志路径、数据来源。
- 澄清：复现目标（完全复现/近似复现）、时间预算、允许误差范围。

## 方法论流程
1. Result Identity：定位结果名称、输出文件、对应脚本与配置。
2. Provenance Record：记录 `input | preprocessing | script | version/commit | params | seed | env | output`。
3. Manual Step Audit：检查表格手工改写、筛选、复制粘贴和 notebook 隐式步骤。
4. Missing Metadata Grading：按缺失严重性排序修复。
5. Reproducibility Grade：A/B/C/D 并给 next fixes。

## 输出模板
- Provenance Record Table。
- Missing Items with Severity。
- Manual Step Risk Notes。
- Reproducibility Grade and Rationale。
- Next Fixes with Owner and ETA。

## 质量门槛
- 不能倒推未知命令或杜撰参数。
- 区分“文件事实 / 用户补充 / 模型推断”三类信息。
- 缺失关键元数据时不得给 A 级。
- 输出必须可用于后续自动化脚本化改造。
