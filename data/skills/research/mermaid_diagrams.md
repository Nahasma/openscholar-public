---
name: "mermaid_diagrams"
description: "使用 Mermaid 语法生成流程图、序列图、类图等结构化图表"
category: "research"
tags: ["mermaid", "flowchart", "sequence-diagram", "figure"]
version: 1
author: "system"
when_to_use: "Use when the user needs a fast flowchart, sequence diagram, class diagram, or lightweight architecture diagram."
user-invocable: true
exposure: both
---

# Mermaid 制图技能

Mermaid 是 LLM 生成成功率最高的图表语法，token 效率比 XML/JSON 格式高约 24 倍。

## 适用场景

- 流程图 (flowchart)
- 序列图 (sequenceDiagram)
- 类图 (classDiagram)
- 状态图 (stateDiagram-v2)
- 甘特图 (gantt)
- 思维导图 (mindmap)

## 渲染方式

```bash
# 安装 (首次)
npm install -g @mermaid-js/mermaid-cli

# 渲染
mmdc -i figures/diagram.mmd -o figures/diagram.png -w 1200 -H 800 --backgroundColor white
```

## 代码模板

### 流程图

```mermaid
flowchart TD
    A[Input Data] --> B{Preprocessing}
    B -->|Clean| C[Feature Extraction]
    B -->|Raw| D[Direct Input]
    C --> E[Model Training]
    D --> E
    E --> F[Evaluation]
    F -->|Good| G[Deploy]
    F -->|Bad| B
```

### 序列图

```mermaid
sequenceDiagram
    participant U as User
    participant S as Server
    participant D as Database
    U->>S: Request
    S->>D: Query
    D-->>S: Results
    S-->>U: Response
```

### 架构图

```mermaid
flowchart LR
    subgraph Frontend
        UI[Web UI]
        CLI[CLI Tool]
    end
    subgraph Backend
        API[API Server]
        Worker[Task Worker]
    end
    subgraph Storage
        DB[(Database)]
        Cache[(Redis)]
    end
    UI --> API
    CLI --> API
    API --> DB
    API --> Cache
    API --> Worker
```

## 执行流程

1. 将 Mermaid 代码写入 `figures/xxx.mmd`
2. 执行：`mmdc -i figures/xxx.mmd -o figures/xxx.png -w 1200 --backgroundColor white`
3. 验证输出文件存在

## vs D2 选择

| 特性 | Mermaid | D2 |
|------|---------|-----|
| LLM 生成成功率 | 更高 | 较高 |
| 视觉质量 | 良好 | 更好 |
| Token 效率 | 最优 | 良好 |
| GitHub 渲染 | 原生支持 | 不支持 |
| 适合场景 | 流程图、序列图 | 复杂架构图 |

**建议**：简单图用 Mermaid，复杂嵌套架构图用 D2 (DiagramGen)。
