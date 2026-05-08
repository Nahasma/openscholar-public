---
name: "matplotlib_scientific"
description: "使用 matplotlib + SciencePlots 生成期刊级科研数据可视化图表"
category: "research"
tags: ["matplotlib", "SciencePlots", "data-visualization", "figure"]
version: 1
author: "system"
exposure: implicit
---

# matplotlib 科研数据可视化

使用 matplotlib + SciencePlots 生成期刊级数据图表。**所有数据可视化都应使用此方法**，而非 ImageGen。

## 环境准备

```bash
pip install matplotlib SciencePlots numpy
```

## 代码模板

### 基础结构

```python
import matplotlib.pyplot as plt
import numpy as np

# 使用期刊样式
plt.style.use(['science', 'ieee'])  # 或 'nature', 'grid'

fig, ax = plt.subplots(figsize=(3.5, 2.625))  # IEEE 单栏宽度

# ... 绑定数据和绘图 ...

ax.set_xlabel('X Label')
ax.set_ylabel('Y Label')
ax.legend()
fig.savefig('figures/plot_name.png', dpi=300, bbox_inches='tight')
plt.close()
```

### 柱状对比图

```python
models = ['Model A', 'Model B', 'Model C']
scores = [85.2, 91.7, 88.4]
colors = ['#4e79a7', '#f28e2b', '#e15759']

fig, ax = plt.subplots(figsize=(3.5, 2.625))
bars = ax.bar(models, scores, color=colors, width=0.6)
ax.bar_label(bars, fmt='%.1f')
ax.set_ylabel('Accuracy (%)')
ax.set_ylim(80, 95)
fig.savefig('figures/comparison.png', dpi=300, bbox_inches='tight')
```

### 折线趋势图

```python
x = np.arange(1, 11)
y1 = [...]  # 数据来自论文
y2 = [...]

fig, ax = plt.subplots(figsize=(3.5, 2.625))
ax.plot(x, y1, 'o-', label='Method A')
ax.plot(x, y2, 's--', label='Method B')
ax.set_xlabel('Epoch')
ax.set_ylabel('Loss')
ax.legend()
fig.savefig('figures/trend.png', dpi=300, bbox_inches='tight')
```

## 可用样式

| 样式 | 适用场景 |
|------|----------|
| `['science', 'ieee']` | IEEE 期刊/会议 |
| `['science', 'nature']` | Nature 系列期刊 |
| `['science', 'grid']` | 通用学术，带网格 |
| `['science', 'high-vis']` | 演讲/海报，高对比度 |

## 执行方式

1. 将代码写入 `figures/plot_xxx.py`
2. 通过 Bash 执行：`python3 figures/plot_xxx.py`
3. 检查输出文件是否存在：`ls -la figures/xxx.png`
4. 数据必须来自论文原文，不可编造

## 常见错误避免

- 不要用 `plt.show()`（无头环境会报错）
- 始终用 `fig.savefig()` 保存
- 始终用 `plt.close()` 释放内存
- SciencePlots 未安装时，回退到默认样式
