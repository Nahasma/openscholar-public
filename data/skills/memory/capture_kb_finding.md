---
name: capture_kb_finding
description: "从知识库查询结果中捕获关键发现"
category: "memory"
tags: ["memory", "insert", "kb"]
version: 1
author: "system"
update_type: insert
exposure: none
---
Skill: Capture KB Finding
Purpose: 从知识库查询结果中提取和存储关键事实性发现。
When to use:
- KB 查询返回了具体的数据点（数值、方法、结论）
- 信息在未来写作或分析中可能有用
How to apply:
- 在记忆中包含 paper_id 和页码范围
- 保持每条发现原子化且可验证
- 在 metadata 中标记 source_type: kb_query
Constraints:
- 只存储来自 KB 结果的已验证事实，不存储 LLM 解读
- 必须包含来源归属
Action type: INSERT only.
