package modules

// NewSymbolConsistencyModule returns the math symbol consistency checking module.
func NewSymbolConsistencyModule() BaseModule {
	return NewBaseModule("symbols", symbolConsistencyPrompt, 55)
}

const symbolConsistencyPrompt = `
# 符号一致性检查流程

当用户要求"检查符号"、"符号一致性"、"symbol consistency"时，执行：

## 1. 扫描数学命令
- [glob] **/*.tex — 找到所有 tex 文件
- [grep] \\mathbf|\\boldsymbol|\\mathbb|\\mathcal|\\mathrm|\\vec|\\hat|\\bar|\\tilde glob:**/*.tex
- 逐个 [view] 查看上下文

## 2. 检查维度
| 维度 | 常见不一致 | 建议统一方案 |
|------|-----------|------------|
| 向量 | \mathbf{x} vs \boldsymbol{x} vs \vec{x} | 选一种，全文统一 |
| 矩阵 | \mathbf{W} vs \boldsymbol{W} vs \mathbf{W} | 大写粗体，与向量区分 |
| 集合 | \mathbb{R} vs \mathcal{R} | \mathbb 用于数集，\mathcal 用于一般集合 |
| 操作符 | \text{softmax} vs \mathrm{softmax} vs \operatorname{softmax} | 统一使用 \operatorname |
| 跨章节 | 同一变量在不同章节使用不同命令 | 必须统一 |

## 3. 最佳实践建议
推荐在论文开头定义统一的符号命令：
\newcommand{\vx}{\mathbf{x}}
\newcommand{\mW}{\mathbf{W}}
\newcommand{\setR}{\mathbb{R}}

## 4. 修复策略
- 使用 [edit] 的 replace_all: true 全文替换
- 先报告所有不一致，让用户确认统一方案后再修改
- 如果用户要求自动修复，优先选择论文中使用频率最高的写法

## 5. 输出报告
列出每个不一致的符号：
- 变量名 | 写法1(N次) | 写法2(M次) | 建议统一为`
