package modules

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
)

// ExplorerLeaderModule 开放探索模式 Leader 指令
const ExplorerLeaderModule = `
## 开放探索模式

你正在进行开放式科研探索。与预定义模板不同，你需要根据研究进展动态规划后续阶段。

### 工作方式
1. 从「研究提案」阶段开始，生成 research proposal（.handoff/00-proposal.md）
2. 根据 proposal 内容，在 proposal 中明确给出建议的后续阶段顺序、目标和交付物
3. 典型的阶段序列：
   - 研究提案 → 文献调研 → 方法设计 → 原型实验 → 论文写作 → 审稿修订
   - 研究提案 → 多方向探索 → 聚焦实验 → 论文写作 → 审稿修订
   - 研究提案 → 理论推导 → 验证实验 → 论文写作 → 审稿修订

### 阶段规划输出
- 对每个建议阶段写明：阶段名称、目标、关键问题、交付物、建议并行度、是否需要 checkpoint
- 不要假设存在动态 add_phase 工具；把阶段规划写入 proposal / handoff，并在当前批准的阶段内继续推进

### 约束
- 总阶段数不超过 8
- 最终必须进入已有的「论文写作」→「审稿修订」阶段
- 每个动态阶段必须产出 .handoff/ 文件
- 注意总预算控制
`

// TreeSearchInstructionHeader is the domain-independent orchestration protocol.
const TreeSearchInstructionHeader = `
## Experiment Execution Protocol

For each experiment node, follow this orchestration flow:
1. Use ExperimentBrief to create a task contract and workspace
2. Use ExperimentDispatch to delegate coding to an external agent (Claude Code / Codex CLI)
3. Use ExperimentCollect to verify deliverables
4. Use ExperimentAssess to evaluate results
5. If failed, use ExperimentRecover for diagnosis and retry strategy

**Important**: Do NOT use Bash to run experiment code directly.
Delegate all coding and execution to external agents via ExperimentDispatch.
`

// TreeSearchInstructionFooter is the domain-independent journal and debug rules.
const TreeSearchInstructionFooter = `
### 实验日志
每次 ExperimentAssess 完成后，结果自动写入 experiment-journal.jsonl。

### Debug 规则
执行失败后用 ExperimentRecover 分类失败，再用 ExperimentDispatch 重新派发。最多重试 4 次。
`

// BuildTreeSearchInstruction generates the full tree search instruction with domain-specific stages.
func BuildTreeSearchInstruction(stages []config.StageDescription) string {
	var sb strings.Builder
	sb.WriteString(TreeSearchInstructionHeader)
	for i, stage := range stages {
		fmt.Fprintf(&sb, "\n### Stage %d: %s\n", i+1, stage.Name)
		sb.WriteString(stage.Description)
		sb.WriteString("\n")
	}
	sb.WriteString(TreeSearchInstructionFooter)
	return sb.String()
}

// TreeSearchInstruction is the default ML tree search instruction (backward compatible).
var TreeSearchInstruction = BuildTreeSearchInstruction([]config.StageDescription{
	{Name: "Preliminary Investigation", Description: "创建基线实验（根节点）— ExperimentBrief + ExperimentDispatch\n外部 agent 实现最简版本的代码，验证可运行\nExperimentCollect + ExperimentAssess 验收结果"},
	{Name: "Hyperparameter Tuning", Description: "基于 Stage 1 的结果，探索 2-3 组超参配置\n每组配置作为独立实验节点，通过 ExperimentBrief + ExperimentDispatch 并行执行\n选择最优配置进入下一阶段"},
	{Name: "Research Agenda Execution", Description: "基于最优超参，执行核心研究实验\n每个研究方向作为独立节点，通过 ExperimentDispatch 委派\n选择最优方向进入下一阶段"},
	{Name: "Ablation Studies", Description: "对最优实验做消融分析\n每个消融条件一个节点\n生成 Replication 节点（不同种子）+ Aggregation 节点（汇总 mean±std）"},
})

// DebugRetryInstruction Worker Agent debug 指令
const DebugRetryInstruction = `
## 实验代码 Debug 规则

执行失败后的恢复流程：
1. 使用 ExperimentRecover 分类失败类型
2. 根据恢复策略行动：
   - retry → 直接用 ExperimentDispatch 重新派发
   - switch_provider → 用 ExperimentDispatch 指定替代 provider
   - repair → 用 ExperimentBrief 重新生成合同（加强约束），再派发
   - rollback → 从 snapshot 恢复，重新派发
   - archive → 记录为阴性结果，不视为系统错误
3. 最多重试 4 次（由 contract.RetryPolicy 控制）
4. 每次尝试的结果自动通过 ExperimentAssess 写入 experiment-journal.jsonl
5. 如果 4 次仍失败：标记实验为 buggy，报告给 Leader
`
