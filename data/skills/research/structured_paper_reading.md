---
name: "structured_paper_reading"
description: "结构化精读单篇论文，产出 claim-evidence 台账、实验矩阵、复现分级与可引用结论。"
category: "research"
tags: ["paper-reading", "structured-reading", "claim-evidence", "evidence-ledger", "reproducibility", "论文精读", "结构化精读论文"]
version: 1
author: "system"
when_to_use: "当用户已经锁定一篇具体论文，需要深读与证据化拆解（而非多篇综述或周报汇总）时使用。"
allowed-tools: ["ScholarSearch", "KBTree", "KBQuery", "KBSearch", "View", "AskUser"]
agent: "research"
effort: "high"
user-invocable: true
exposure: implicit
source: "builtin"
---

# 结构化精读论文

## 输入与澄清
- 输入优先级：PDF 路径、KB `paper_id`、标题+年份、DOI、arXiv ID。
- 最多追问 1-2 个关键点：目标论文标识、输出用途（复现/引用/组会/开题）。
- 若用户只给主题，先用 `ScholarSearch` 缩小候选，再让用户确认单篇目标。

## 方法论流程
1. 身份核验：题名、作者、年份、venue、DOI/arXiv、版本一致性。
2. 分层阅读：
- Pass 1：题名/摘要/图表/结论，判断是否值得深读。
- Pass 2：方法/实验/局限，抽取核心 claim 与证据位置。
- Pass 3：复现条件、可引用结论、与用户课题连接。
3. Claim-Evidence Ledger：每条 claim 都绑定证据定位（section/page/figure/table/KB node）。
4. Experiment Matrix：任务、数据、指标、基线、设定差异、主要结果与失败案例。
5. Reproducibility Triage：代码、数据、license、环境、参数、seed、期望指标、阻塞项分级。
6. 反例与矛盾证据：列出与 claim 不一致、或需要二次验证的证据。

## 输出模板
- Paper Card：基本信息、一句话结论、研究问题、核心贡献。
- Claim-Evidence Ledger：`claim | locator | evidence type | confidence | caveat`。
- Experiment Matrix：`setting | baseline | metric | result | limitation`。
- Reproducibility Manifest：`code/data/env/params/seed/expected metric/blocker`。
- Citation-ready Claims：仅保留有定位依据的可引用结论。
- Relation to My Project：可用点、不可用点、待验证问题。

## 质量门槛
- 每个数字、结论、引文必须给 locator；没有就标“待确认”。
- 未读原文只能输出 candidate insight，不给确定性结论。
- 不得补造 citation count、venue 信息、实验数值。
- 无法核验证据时明确降级为“需要验证”。
