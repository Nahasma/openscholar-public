---
name: "research_weekly_report"
description: "将最近科研活动整理为证据化周报：对比上周计划差异、沉淀决策/阻塞/行动项并标注 owner 与 due date。"
category: "workflow"
tags: ["weekly-report", "progress-report", "evidence-archive", "lab-sync", "owner-due-date", "科研周报"]
version: 1
author: "system"
when_to_use: "当用户要汇总最近一周科研进展并形成导师/组会/个人复盘报告时使用，不用于单篇论文分析。"
allowed-tools: ["KBList", "KBSearch", "KBQuery", "Glob", "Grep", "View", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 科研周报与进展同步

## 输入与时间窗
- 默认时间窗：最近 7 个自然日；若用户给日期则覆盖。
- 组会场景可切换为“上次组会后至今日”。
- 最多追问 3 点：汇报对象、输出语言、是否有上周计划基线。

## 方法论流程
1. Evidence Harvest：汇总用户输入、workspace 文件、KB 阅读记录、实验与写作产物。
2. Delta 对比：对照上周计划，标记完成/变更/延期/取消。
3. 事件分类：
- fact：可验证已发生事项。
- decision：已做决定及依据。
- blocker：阻塞与影响范围。
- action：下一步动作。
- request：需要导师或合作者反馈。
4. Action Structuring：每个动作补 `owner | due date | dependency | risk`。
5. Stakeholder 变体：导师邮件版、组会口播版、个人复盘版。

## 输出模板
- This Week Delta（相对上周计划的变化）。
- Evidence-backed Progress（阅读/实验/写作/协作）。
- Decisions and Rationales。
- Blockers and Impact。
- Next Week Plan（含 owner 与 due date）。
- Requests for Feedback。

## 质量门槛
- 每条进展必须有证据来源或标“用户待确认”。
- 禁止“持续推进”等无证据填充句。
- 每个 action 必须有 owner，若缺失则显式待用户确认。
- 仅做组织与总结，不伪造产出或结论。
