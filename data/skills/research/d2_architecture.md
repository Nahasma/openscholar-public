---
name: "d2_architecture"
description: "D2 架构图高级用法：嵌套容器、样式自定义、主题选择等最佳实践"
category: "research"
tags: ["d2", "architecture", "DiagramGen", "figure", "系统架构图", "画一个系统架构图"]
version: 1
author: "system"
when_to_use: "Use when the user needs a polished architecture diagram with nested systems, theming, or richer visual styling."
user-invocable: true
exposure: both
---

# D2 架构图最佳实践

D2 通过 DiagramGen 工具渲染，输出 SVG+PNG，适合复杂的嵌套架构图。
DiagramGen 默认会保存 `.d2` 源码并输出 SVG（可选 PNG），成功后直接汇报路径即可，不需要再跑 `ls`/`which d2`/`d2 --help`。

## 调用方式

使用 DiagramGen 工具：
- `code`：D2 语法代码
- `filename`：输出文件名（不含扩展名）
- `theme`：主题 ID（推荐 3=flagship 用于论文；0 表示自动）
- `format`：`both|svg|png`（建议 `both`）
- `style_preset`：`paper|minimal|none`（默认 `paper`）
- `strict_quality`：默认 true，歧义 nested 引用会报错

## 高级语法

### 嵌套容器（子系统）

```d2
direction: right

Encoder: Encoder Stack {
  MHA: Multi-Head Attention
  FFN: Feed-Forward Network
  Norm1: Layer Norm
  Norm2: Layer Norm

  MHA -> Norm1 -> FFN -> Norm2
}

Decoder: Decoder Stack {
  MaskedMHA: Masked Multi-Head Attention
  CrossMHA: Cross Attention
  FFN: Feed-Forward Network
}

Input -> Encoder
Encoder -> Decoder.CrossMHA
Decoder -> Output
```

> 规则：嵌套节点连线必须用全限定引用（如 `Decoder.CrossMHA`），不要写裸 ID（如 `CrossMHA`），否则容易生成漂浮顶层节点。

### 样式自定义

```d2
node: {
  style: {
    fill: "#E8F4FD"
    stroke: "#2196F3"
    border-radius: 8
    font-size: 14
  }
}

important_node: Critical Component {
  style: {
    fill: "#FFEBEE"
    stroke: "#F44336"
    stroke-width: 2
    bold: true
  }
}
```

> `stroke-dash` 必须用数值（如 `0` 或 `3`），不要写 `true/false`。

### 形状选择

- `shape: rectangle` — 默认，模块/组件
- `shape: oval` — 开始/结束节点
- `shape: diamond` — 决策节点
- `shape: cylinder` — 数据库/存储
- `shape: hexagon` — 中间件/服务
- `shape: queue` — 消息队列

## 主题推荐

| 主题 ID | 名称 | 论文适用性 |
|---------|------|-----------|
| 0 | Default | 通用 |
| 3 | Flagship Terrastruct | 推荐用于论文（清爽专业） |
| 4 | Cool classics | 蓝色调，适合技术文档 |
| 100 | Dark Macaroni | 深色背景演讲用 |

## 最佳实践

1. 使用 `direction: right` 或 `direction: down` 明确布局方向
2. 用嵌套 `{}` 表示子系统边界
3. 保持节点标签简短（2-4 个词）
4. 用颜色区分不同层次/类型的组件
5. 复杂图拆分为多个子图，而非堆积在一张图中
