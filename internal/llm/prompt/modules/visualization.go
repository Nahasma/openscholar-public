package modules

// NewVisualizationModule returns the visualization tool selection strategy module.
func NewVisualizationModule() BaseModule {
	return NewBaseModule("visualization", visualizationPrompt, 37)
}

const visualizationPrompt = `
# 可视化策略

论文插图根据图表类型选择不同工具生成。核心原则：**精确内容用代码渲染，概念内容用 AI 生成**。
可通过 SkillQuery 搜索 "figure" 获取各方法的详细模板和最佳实践。

## 图类型分流策略（核心规则）

| 图表类型 | 工具 | 适用场景 |
|----------|------|----------|
| 架构图/系统组件图 | **DiagramGen (D2)** | 模块关系、数据流、层次结构 |
| 流程图/决策树 | **DiagramGen (D2)** 或 **Mermaid (Bash)** | 算法步骤、工作流程 |
| 序列图/时序图 | **Mermaid (Bash)** | 交互时序、消息传递 |
| 数据可视化 | **matplotlib + SciencePlots (Bash)** | 柱状图、折线图、散点图、热力图 |
| 数学/几何示意图 | **TikZ** | 公式推导、几何关系 |
| 概念场景图 | **ImageGen (AI)** | 应用场景示意、抽象概念 |
| 分类树/层次图 | **DiagramGen (D2)** | 概念分类、知识体系 |

### 为什么数据可视化不用 ImageGen？
AI 图像生成模型无法保证数据精确性——生成的柱状图/折线图中的数值、比例关系完全不可靠。
matplotlib 代码生成的图表数据 100% 精确，且支持 SciencePlots 期刊级样式。

### 为什么技术图不用 ImageGen？
AI 图像生成模型对包含精确文字的技术图生成质量差，文字容易出现拼写错误和乱码。
DiagramGen 使用 D2 代码渲染，文字由矢量引擎精确排版，不会出现乱码。

## 配图需求评估（主动触发）

写作过程中，**主动评估**每个 section 是否需要配图：

- **架构 / 方法类 section**：用 **DiagramGen** 生成架构图、流程图
- **实验 / 评估类 section**：用 **matplotlib** 生成性能对比图、数据可视化
- **应用场景类 section**：用 **ImageGen** 生成场景示意图（仅概念图）
- **历史演进 / 概览类 section**：用 **DiagramGen** 生成时间线、层次分类图

评估标准：如果一个 section 超过 1 页但没有任何图表，**必须**添加配图增强可读性。

## 强制规则
- 每篇论文至少 3 张图（架构图 + 数据图 + 其他）
- 全文写完后如果配图不足，在自审阶段补充
- **禁止**用 ImageGen 生成数据可视化图（柱状图、折线图等）— 必须用 matplotlib
- **禁止**用 ImageGen 生成包含精确文字标注的技术图 — 必须用 DiagramGen 或 Mermaid
- ImageGen **仅用于**概念场景图和抽象示意图
- TikZ 仅限数学/几何图和简单结构图

## 生成后验证（必须执行）

每次生成图片后：
1. **代码生成类**（matplotlib/D2/Mermaid/TikZ）：检查工具返回的保存路径与成功信息即可；避免再跑 ls/which/--help/glob 这类低价值检查命令
2. **AI 生成类**（ImageGen）：告知用户 "此图为 AI 生成，建议人工检查准确性"
3. 工具成功后直接进入收尾：回复已保存路径并继续正文，不做额外 shell 验证
4. 如果生成失败，按降级路径重试：D2 → Mermaid → 文字描述

## matplotlib 数据可视化（推荐方式）

适用于需要精确数据的图表。步骤：
1. 生成 Python 脚本（使用 SciencePlots 样式）
2. 通过 Bash 执行脚本
3. 确认输出图片文件存在
4. 数据必须来自论文原文或权威 benchmark，不可编造

样式选择：
- IEEE 论文：plt.style.use(['science', 'ieee'])
- Nature 论文：plt.style.use(['science', 'nature'])
- 通用学术：plt.style.use(['science', 'grid'])

如果 SciencePlots 未安装，回退到 matplotlib 默认样式。

## Mermaid 制图（流程图/序列图推荐）

适用于流程图和序列图，LLM 生成成功率最高。步骤：
1. 将 Mermaid 代码写入 .mmd 文件
2. 通过 Bash 执行：mmdc -i <input>.mmd -o <output>.png -w 1200 --backgroundColor white
3. 确认输出文件存在

## DiagramGen 工具使用方式

调用示例：
[DiagramGen] code="direction: right\nEncoder: Encoder {\n  MHA: Multi-Head Attention\n  FFN: Feed-Forward\n}\nDecoder: Decoder {\n  CrossAttn: Cross Attention\n}\nEncoder -> Decoder" filename="architecture" theme=3

参数：
- code（必填）：D2 图表语法代码
- filename（必填）：输出文件名（不含扩展名）
- theme（可选）：主题 ID（0=自动, 3=flagship 推荐论文用）
- format（可选）："both|svg|png"，建议 "both"
- style_preset（可选）："paper|minimal|none"，默认 "paper"
- strict_quality（可选）：默认 true；出现歧义 nested 引用时返回可修复错误

D2 质量要求：
- 嵌套节点连线必须使用全限定引用（如 "Encoder.MHA -> Decoder.CrossAttn"），禁止裸子节点 ID
- style.stroke-dash 使用数值（如 "0"、"3"），不要写布尔值
- 论文图默认白底、低杂讯布局

## ImageGen 工具使用方式（仅限概念图）

调用示例：
[ImageGen] prompt="A clean scientific illustration of neural network learning process, minimalist style, white background, no text labels" filename="concept_illustration"

参数：
- prompt（必填）：英文描述，避免要求精确文字
- filename（必填）：输出文件名

Prompt 编写要求：
- 使用英文描述
- 默认指定 white background（适合论文；若明确需要 dark background 可覆盖）
- 明确指定风格：scientific illustration / minimalist
- **必须声明 "no text, no labels, no annotations"**（AI 模型生成的文字几乎都会乱码）
- 系统会自动增强 prompt（追加防乱码指令 + 白底低杂讯约束 + 学术风格），无需重复添加

### 混合方案：AI 底图 + LaTeX 标注

当概念图**必须包含文字标注**时，使用两步法：
1. 用 ImageGen 生成**无文字底图**（prompt 中写明 no text）
2. 用 TikZ \node 在 LaTeX 中叠加文字标注：在 tikzpicture 环境中 \includegraphics 底图，然后用 scope + \node 在图上叠加精确文字

优势：文字由 LaTeX 精确渲染（不会乱码），底图由 AI 生成（视觉丰富）。详见 SkillQuery 搜索 "imagegen" 获取完整模板。

## 数据来源规则
所有图表中的数据必须来自原论文或权威来源，不可凭训练知识编造：
- 搜索 Semantic Scholar 获取原论文数据
- 在代码或 prompt 中使用论文中的确切数值
- 在图的 caption 中注明数据来源`
