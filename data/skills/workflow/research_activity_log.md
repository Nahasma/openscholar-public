---
name: "research_activity_log"
description: "将阅读、实验、会议和失败尝试沉淀为可追溯科研日志，支持后续周报与复盘。"
category: "workflow"
tags: ["lab-notebook", "activity-log", "research-log", "meeting-notes", "科研日志"]
version: 1
author: "system"
when_to_use: "当用户要记录日常科研活动（实验、会议、阅读、决策、失败）并建立可追溯日志时使用。"
allowed-tools: ["Glob", "Grep", "View", "KBSearch", "KBQuery", "AskUser"]
agent: "research"
effort: "medium"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 科研活动日志

## 输入与澄清
- 输入：日期范围、活动类型、相关项目/论文/实验、原始记录文件。
- 澄清：记录粒度（简要条目/完整实验日志）、主要读者（自己/导师/团队）。

## 方法论流程
1. Entry Header：`date | subject | activity type | participants | project`。
2. Protocol Record：做了什么、为何做、用到的数据/代码、结果与失败。
3. Linkage：关联文件、KB paper_id、实验 ID、后续任务。
4. Rollup Fields：抽取 decision/blocker/action/request 供周报复用。
5. Consistency Check：缺时间、缺主题、缺证据链接的条目标红。

## 输出模板
- Activity Entry。
- Evidence Links。
- Decisions。
- Action Items。
- Open Questions。

## 质量门槛
- 每条日志必须有 date 和 subject。
- 保留失败与不确定性，不改写成“全是成功”的总结。
- 不伪造会议结论或实验结果。
- 日志与周报分离：日志保留过程细节，周报另行汇总。
