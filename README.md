# OpenScholar

面向学术论文搜索、阅读和知识库问答的终端 AI 助手。长期目标是覆盖 CS 科研人员从文献调研、论文阅读、写作修改到实验协作的全流程。

> 项目状态：Alpha。当前较稳定的是 **论文搜索、PDF 阅读、知识库导入和带页码引用的论文问答**；写作辅助、格式转换、科研团队、多 Agent 编排和技能自进化仍在施工中。CLI、配置格式、科研流水线与内置工具协议在 `v1.0.0` 前可能继续调整。

## 项目状态与发布通道

OpenScholar 目前处于 **Alpha** 阶段，优先验证论文搜索/阅读体验、知识库问答链路和工具体系。欢迎早期试用，但不建议直接用于不可回滚的生产流程。

| 通道 | 版本示例 | 适用场景 | 稳定性承诺 |
|------|----------|----------|------------|
| Alpha | `v0.1.0-alpha.2` | 早期试用、功能反馈、集成验证 | 可能有破坏性变更 |
| Beta | `v0.1.0-beta.1` | 更广泛试用、文档和兼容性验证 | 核心 CLI/TUI 行为基本稳定 |
| Stable | `v1.0.0` | 日常使用和下游集成 | 遵循 SemVer，避免破坏性变更 |

发布预览版时，GitHub Release 应标记为 pre-release；npm 发布建议使用对应 dist-tag：

```bash
npm publish --tag alpha
npm publish --tag beta
```

在 `v1.0.0` 前，配置文件结构、斜杠命令、内置工具参数、科研流水线编排和数据库迁移仍可能调整。升级前建议备份项目内 `.openscholar/` 目录。

## 功能特性

| 功能 | 当前状态 | 说明 |
|------|----------|------|
| 文献搜索 | 稳定 | 支持 Semantic Scholar、OpenAlex、Crossref、PubMed、arXiv 等来源；适合关键词检索、论文详情、引用/参考文献扩展 |
| PDF 阅读 | 稳定 | 直接读取 PDF 指定页范围，长 PDF 会提示分页阅读，适合快速定位公式、实验和结论 |
| 论文知识库 | 稳定 | 支持 PDF 导入、本地 SQLite 索引、FTS 检索、按页码引用回答 |
| 论文问答 | 稳定 | `/ask-paper` 和知识库查询可基于论文内容生成带来源页码的回答 |
| 多模型接入 | 可用 | 支持 Anthropic、OpenAI、DeepSeek、MiniMax、GLM、SiliconFlow、Ollama 和 OpenAI-compatible provider |
| TUI 交互 | 可用 | Bubble Tea 终端界面、权限模式、工具调用展示、会话状态和基础快捷键 |
| 论文模板 | 可用 | 内置 NeurIPS / ICML / CVPR / ACL / AAAI / ICLR / ECCV / IEEE / KDD / arXiv 模板 |
| 对话式写作 | 施工中 | 可生成和编辑 LaTeX/Markdown，但工作流和质量门禁还在收敛 |
| 文档格式转换 | 施工中 | 基于 pandoc 的 LaTeX / Word / Markdown / PDF 转换，依赖本机工具链 |
| 科研绘图 | 施工中 | AI 图像生成和 D2 技术图能力已接入，输出质量仍需人工审核 |
| 多 Agent 科研团队 | 实验中 | Leader-Worker 编排、研究流水线和预算控制仍在快速迭代 |
| 技能自进化 | 实验中 | MemSkill、SkillBank 和反馈反思 Pipeline 可用，但接口和治理模型可能变化 |

## 当前推荐使用场景

- 搜索某个研究方向的相关论文，继续追踪引用和参考文献。
- 把 PDF 加入本地知识库，然后围绕方法、公式、实验设置、消融结果进行问答。
- 在 TUI 中结合文献检索、网页检索和本地论文内容做快速阅读笔记。
- 用内置模板初始化论文项目，但写作和自动化投稿前仍建议人工审阅所有输出。

## 快速开始

### 安装条件

基础使用需要：

- Go 1.23 或更高版本
- Git
- Python 3，可用于 PDF/文档解析脚本
- 至少一个 LLM Provider API Key，或本地 Ollama 模型

可选能力需要：

| 能力 | 依赖 | 说明 |
|------|------|------|
| 增强 PDF 树索引 | PageIndex 及其 Python 依赖 | 未安装时会退回基础解析模式 |
| 文档导出/转换 | pandoc | 用于 LaTeX / Markdown / DOCX 转换 |
| PDF 渲染导出 | LaTeX 发行版或 weasyprint | 仅在需要生成 PDF 时安装 |
| 前端开发 | Node.js 20+ | 仅修改 `web/` 时需要 |
| 数据库代码生成 | sqlc | 仅开发数据库查询时需要 |
| 代码检查 | golangci-lint | 仅本地 lint 时需要 |

### 安装

当前公开预览仓库为 `Nahasma/openscholar-public`。Alpha 阶段建议固定版本安装：

```bash
go install github.com/Nahasma/openscholar-public@v0.1.0-alpha.2
```

或从源码构建：

```bash
git clone https://github.com/Nahasma/openscholar-public.git
cd openscholar-public
go build -o openscholar .
```

npm 安装通道准备好后可使用：

```bash
npm install -g openscholar@alpha
```

### 配置 API Key

```bash
# 环境变量（三选一）
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
export DEEPSEEK_API_KEY=sk-...
```

或复制配置文件进行完整配置（Agent 模型、MCP 服务器、科研团队等）：

```bash
cp config.example.json .openscholar/config.json
# 编辑 .openscholar/config.json 填入 API Key 和模型选择
```

### 可选依赖

PageIndex 树索引是知识库的可选增强。未安装 `pageindex` 时，知识库会退回基础解析模式；要启用完整树索引，请按 [PageIndex](https://github.com/VectifyAI/PageIndex) 项目说明安装 Python 依赖。

如果使用文档导出能力，请先安装 `pandoc`：

```bash
# macOS
brew install pandoc

# Ubuntu / Debian
sudo apt install pandoc
```

### 使用

```bash
# 初始化论文项目
mkdir my-paper && cd my-paper
openscholar init --template neurips2026

# 交互模式（TUI）
openscholar

# 搜索论文
openscholar -p "搜索 sparse attention 近三年的代表论文，并给出每篇的贡献摘要"

# 知识库操作
openscholar
> /kb-add ~/papers/attention-is-all-you-need.pdf
> /ask-paper 1706.03762 "multi-head attention 的公式是什么？"
> /kb-list
```

### 科研团队

科研团队功能仍处于实验阶段，适合试用和反馈，不建议依赖它生成最终研究结论。

```bash
openscholar
> /research start "Transformer 在蛋白质结构预测中的应用"
# Leader Agent 自动编排: 文献调研 → 研究设计 → 实验 → 写作 → 审稿

> /research start "多模态学习综述" --template survey --mode auto --budget 10
# 综述模板 + 全自动模式

> /research status    # 查看进度
> /research cost      # 查看费用
```

## 可用模板

| 模板 | 说明 |
|------|------|
| `neurips2026` | NeurIPS 2026 |
| `icml2026` | ICML 2026 |
| `cvpr2026` | CVPR 2026 |
| `acl2026` | ACL 2026 |
| `aaai2026` | AAAI 2026 |
| `iclr2026` | ICLR 2026 |
| `eccv2026` | ECCV 2026 |
| `ieee` | IEEE Trans. |
| `kdd2026` | KDD 2026 |
| `arxiv` | arXiv 预印本 |

## 工具系统

工具分两层：核心工具始终可用，扩展工具通过 `ToolSearch` 按需激活。

### 核心工具（始终加载）

| 工具 | 说明 |
|------|------|
| `View` | 读取文件内容（支持行号、偏移、限制） |
| `Edit` | 精确字符串替换，输出 unified diff |
| `Write` | 创建或覆写文件 |
| `Bash` | 执行 shell 命令（危险命令需确认） |
| `Glob` | 按模式搜索文件（支持 `**`） |
| `Grep` | 正则搜索文件内容 |
| `ToolSearch` | 搜索并激活扩展工具 |

### 扩展工具（按需激活）

| 工具 | 说明 |
|------|------|
| `ScholarSearch` | Semantic Scholar API 文献检索 |
| `KBAdd` | 将论文 PDF 加入知识库（PageIndex 树索引） |
| `KBQuery` | 对知识库论文进行精确问答（树搜索 + 页码引用） |
| `KBList` | 列出知识库中所有论文 |
| `KBTree` | 显示论文的树结构 |
| `ImageGen` | AI 图像生成（GLM CogView / MiniMax / DALL-E） |
| `DiagramGen` | D2 图表语言技术图生成 |
| `PaperValidate` | 论文质量验证（占位符、引用、重复、图表） |
| `Task` | 生成子 Agent 执行并行任务（支持 model 参数覆盖） |
| `DocExport` | 文档格式双向转换（tex↔docx↔md↔pdf，pandoc） |
| `AskUser` | 向用户请求模糊指令确认 |
| `RecordFeedback` | 记录用户反馈，自动触发技能进化 |
| `SkillQuery` | 按需搜索技能库（分类/标签/全文） |
| `SkillManage` | 创建/更新/删除技能 |

## 项目结构

```
openscholar/
├── main.go                     # 入口
├── cmd/                        # CLI 命令 (Cobra)
│   ├── root.go                 # 主命令 + TUI/非交互模式
│   ├── init.go                 # 模板初始化
│   └── auth.go                 # 认证管理
├── internal/
│   ├── app/                    # 应用组合根
│   ├── config/                 # 配置加载
│   ├── db/                     # SQLite + goose 迁移 + sqlc（14 张表）
│   ├── session/                # 会话管理
│   ├── message/                # 消息模型 + 序列化
│   ├── permission/             # 权限请求/授权
│   ├── pubsub/                 # 泛型事件总线
│   ├── bib/                    # 文献引用管理
│   ├── kb/                     # 知识库（PageIndex 树索引 + 检索）
│   ├── memory/                 # MemSkill 记忆系统（技能 + 执行器）
│   ├── skillbank/              # 通用技能库（FTS5 搜索 + .md 文件存储）
│   ├── evolution/              # 技能反思进化（反馈采集 + 三阶段 Pipeline）
│   ├── research/               # 科研团队 Pipeline（Leader-Worker 编排）
│   ├── template/               # 模板嵌入 + 初始化
│   ├── llm/
│   │   ├── models/             # 模型定义 (Claude, GPT, DeepSeek, MiniMax, GLM, SiliconFlow, Ollama)
│   │   ├── provider/           # LLM 提供商适配层（含重试机制）
│   │   ├── agent/              # Agent 执行循环 + Deferred Tools
│   │   ├── prompt/             # 系统提示词（31 个模块化组件）
│   │   └── tools/              # 22 工具 + DeferredRegistry + ToolSearch
│   ├── mcp/                    # MCP Client + 工具桥接
│   ├── testutil/               # 测试工具包（MockProvider + InMemoryDB）
│   └── tui/                    # BubbleTea 终端 UI
│       └── components/         # 聊天/状态栏/权限对话框/diff
├── data/
│   └── memory-skills/          # MemSkill 种子技能（.md 格式）
├── scripts/
│   └── run_pageindex.py        # PageIndex Python 入口
└── templates/                  # 10 个 LaTeX 会议模板
```

## TUI 界面

交互体验对标 [Claude Code](https://claude.ai/code)，基于 Bubble Tea + Lip Gloss + Glamour。

```
┌─────────────────────────────────────────┐
│  ⏺ 用户消息                             │  Viewport
│  ⏺ 助手回复（Markdown 渲染）            │
│  ⏺ KBQuery(1706.03762)                  │
│  ⎿  Answer: Multi-head attention...     │
├─────────────────────────────────────────┤
│  > 输入区（1~8 行自适应）                │  Input
├─────────────────────────────────────────┤
│  ? shortcuts  ctrl+↑↓ scroll    default │  StatusBar
└─────────────────────────────────────────┘
```

### 快捷键

| 按键 | 功能 |
|------|------|
| `Enter` | 发送消息 |
| `Alt+Enter` | 插入换行 |
| `Shift+Tab` | 切换权限模式（default → auto → plan） |
| `Tab` | 展开/折叠工具调用 |
| `Esc` | 取消当前处理 |
| `Ctrl+C` | 退出 |

### 权限模式

- **default** — 读取自动放行，写入/执行需确认
- **auto** — 全部自动放行
- **plan** — 所有操作均需确认

### 斜杠命令

| 命令 | 说明 |
|------|------|
| `/help` | 查看帮助 |
| `/model` | 切换模型 |
| `/clear` | 清除对话 |
| `/compact` | 压缩上下文 |
| `/status` | 会话状态 |
| `/cost` | Token 用量和费用 |
| `/agent` | 切换 Agent |
| `/kb-add` | 导入论文到知识库 |
| `/ask-paper` | 对知识库论文提问 |
| `/kb-list` | 列出知识库论文 |
| `/kb-tree` | 显示论文树结构 |
| `/research start` | 启动科研流水线 |
| `/research status` | 查看流水线进度 |
| `/evolve` | 手动触发技能进化 |
| `/skill-stats` | 查看技能使用统计 |

## 技术栈

- **语言**: Go 1.23+
- **CLI**: [Cobra](https://github.com/spf13/cobra)
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **数据库**: SQLite ([go-sqlite3](https://github.com/ncruces/go-sqlite3)) + [goose](https://github.com/pressly/goose) + [sqlc](https://sqlc.dev)
- **LLM SDK**: [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) / [openai-go](https://github.com/openai/openai-go)
- **知识库**: [PageIndex](https://github.com/VectifyAI/PageIndex) 树状推理检索

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Nahasma/openscholar-public&type=Date)](https://www.star-history.com/#Nahasma/openscholar-public&Date)

## 开发

```bash
go mod tidy          # 安装依赖
sqlc generate        # 生成数据库代码
go build -o openscholar .  # 构建
go test ./...        # 运行测试
./restart.sh         # 重启服务
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
