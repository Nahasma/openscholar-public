package research

// PhaseTemplate 定义阶段模板
type PhaseTemplate struct {
	Name         string
	Checkpoint   bool
	MaxWorkers   int
	Deliverables []string // 期望的交付物文件 glob 模式（相对于工作目录）
}

// Templates 内置阶段模板，Leader 根据主题自动选择或用户 --template 指定
var Templates = map[string][]PhaseTemplate{
	"empirical": {
		{Name: "文献调研", Checkpoint: true, MaxWorkers: 3, Deliverables: []string{".handoff/01-literature.md"}},
		{Name: "研究设计", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/02-design.md"}},
		{Name: "实验实施", Checkpoint: true, MaxWorkers: 3, Deliverables: []string{".handoff/03-experiment.md", "code/**/*.py"}},
		{Name: "论文写作", Checkpoint: false, MaxWorkers: 3, Deliverables: []string{"paper/sections/*.tex"}},
		{Name: "审稿修订", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/04-review.md"}},
	},
	"aris_empirical": {
		{Name: "Idea Discovery", Checkpoint: true, MaxWorkers: 3, Deliverables: []string{"idea-stage/IDEA_REPORT.md", "idea-stage/IDEA_CANDIDATES.md", ".handoff/01-literature.md"}},
		{Name: "Method Refinement", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{"refine-logs/FINAL_PROPOSAL.md", "refine-logs/REFINE_STATE.json", ".handoff/02-design.md"}},
		{Name: "Experiment Planning", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{"refine-logs/EXPERIMENT_PLAN.md", "refine-logs/EXPERIMENT_TRACKER.md", ".handoff/03-plan.md"}},
		{Name: "Experiment Execution", Checkpoint: true, MaxWorkers: 3, Deliverables: []string{"experiments/nodes/*/manifest.json", "experiment-journal.jsonl", ".handoff/04-experiment.md"}},
		{Name: "Claim & Integrity Gate", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{"EXPERIMENT_AUDIT.json", "CLAIMS_FROM_RESULTS.json", ".handoff/05-claims.md"}},
		{Name: "Auto Review Loop", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{"review-stage/AUTO_REVIEW.md", "review-stage/REVIEW_STATE.json", ".handoff/06-review.md"}},
		{Name: "Writing Handoff", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{"NARRATIVE_REPORT.md", ".handoff/07-writing.md"}},
	},
	"survey": {
		{Name: "文献调研", Checkpoint: true, MaxWorkers: 4, Deliverables: []string{".handoff/01-literature.md"}},
		{Name: "分类体系", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/02-design.md"}},
		{Name: "论文写作", Checkpoint: false, MaxWorkers: 3, Deliverables: []string{"paper/sections/*.tex"}},
		{Name: "审稿修订", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/04-review.md"}},
	},
	"theoretical": {
		{Name: "文献调研", Checkpoint: true, MaxWorkers: 3, Deliverables: []string{".handoff/01-literature.md"}},
		{Name: "理论构建", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/02-design.md"}},
		{Name: "论文写作", Checkpoint: false, MaxWorkers: 3, Deliverables: []string{"paper/sections/*.tex"}},
		{Name: "审稿修订", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/04-review.md"}},
	},
	"exploratory": {
		{Name: "研究提案", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/00-proposal.md"}},
		{Name: "论文写作", Checkpoint: false, MaxWorkers: 3, Deliverables: []string{"paper/sections/*.tex"}},
		{Name: "审稿修订", Checkpoint: true, MaxWorkers: 2, Deliverables: []string{".handoff/99-review.md"}},
	},
}
