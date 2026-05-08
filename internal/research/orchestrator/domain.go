package orchestrator

import "github.com/Nahasma/openscholar-public/internal/config"

// DomainPresets contains built-in domain profiles for common scientific disciplines.
// Use "general" as the default for unknown or unspecified domains.
//
// Second-level mapping (documentation, not code):
//   deep_learning, reinforcement_learning, timeseries, recommender, cv → ml
//   llm, rag, text_mining, information_retrieval, dialogue → nlp
//   cfd, fem, electromagnetics, acoustics, multiphysics → simulation
//   dft, drug_design, materials_science, molecular_dynamics → molecular
//   genomics, proteomics, single_cell, systems_biology → bioinformatics
//   causal_inference, bayesian, experimental_design → statistics
//   social_science, policy_evaluation, panel_data → econometrics
//   operations_research, scheduling, combinatorial → optimization
//   networking, databases, compiler, security → systems
//   control, signal_processing, communications, circuits → engineering
//   geoscience, oceanography, atmospheric, remote_sensing → climate
var DomainPresets = map[string]config.DomainProfile{
	// ── general: 领域中性默认，适用于未分类的计算实验 ──
	"general": {
		Name:               "general",
		Languages:          []string{},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md"},
		AcceptCriteria:     []string{"代码或工作流可复现", "关键结果文件存在", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"primary_metric": 0.85, "secondary_metric": 0.72, "runtime_sec": 300}`,
		NodeProgression:    []string{"variant", "analysis", "replication", "aggregation"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Experiment", Description: "创建基线实验 — 验证基本流程可运行\n记录参考指标"},
			{Name: "Variant Exploration", Description: "系统探索关键变量\n每组变体作为独立节点并行执行"},
			{Name: "Analysis & Sensitivity", Description: "对最优变体做深入分析\n敏感性检验和参数影响评估"},
			{Name: "Replication & Aggregation", Description: "独立复现验证\n汇总统计（mean±std）"},
		},
	},

	// ── ml: 机器学习 / 深度学习 / CV / 推荐 / 强化学习 ──
	"ml": {
		Name:               "ml",
		Languages:          []string{"python"},
		Deliverables:       []string{"outputs/metrics.json", "outputs/run_report.md"},
		AcceptCriteria:     []string{"代码可运行", "outputs/metrics.json 存在且包含预期指标", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/metrics.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"accuracy": 0.92, "loss": 0.14, "f1": 0.89, "runtime_sec": 120}`,
		NodeProgression:    []string{"hyperparameter", "research", "ablation", "replication"},
		RootNodeType:       "preliminary",
		Stages: []config.StageDescription{
			{Name: "Preliminary Investigation", Description: "创建基线模型，验证训练流程可运行\n记录基础指标"},
			{Name: "Hyperparameter Tuning", Description: "探索 2-3 组超参配置\n每组配置作为独立节点并行执行\n选择最优配置"},
			{Name: "Research Agenda Execution", Description: "基于最优超参，执行核心研究实验\n每个研究方向作为独立节点"},
			{Name: "Ablation Studies", Description: "消融分析 + Replication（不同种子）\n汇总 mean±std"},
		},
	},

	// ── nlp: 自然语言处理 / 信息检索 / LLM 评估 / RAG ──
	"nlp": {
		Name:               "nlp",
		Languages:          []string{"python"},
		Deliverables:       []string{"outputs/metrics.json", "outputs/run_report.md", "outputs/predictions.jsonl"},
		AcceptCriteria:     []string{"代码可运行", "outputs/metrics.json 存在且包含评估指标", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/metrics.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"bleu": 32.5, "rouge_l": 0.41, "mrr": 0.78, "f1": 0.85, "human_agreement": 0.72}`,
		NodeProgression:    []string{"model_comparison", "ablation", "error_analysis", "human_eval"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Models", Description: "复现基线，建立评估 pipeline\n确认数据加载和指标计算正确"},
			{Name: "Model Comparison", Description: "对比不同模型/方法\n每个模型作为独立节点"},
			{Name: "Ablation & Error Analysis", Description: "消融实验 + 错误分析\n定位模型弱点"},
			{Name: "Human Evaluation", Description: "人工评估（如适用）\n计算评估者一致性"},
		},
	},

	// ── simulation: CFD / FEM / 电磁 / 声学 / 多物理场 / 量子数值模拟 ──
	"simulation": {
		Name:               "simulation",
		Languages:          []string{"python", "openfoam"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/convergence.csv"},
		AcceptCriteria:     []string{"仿真可运行且正常收敛", "outputs/results.json 存在且包含预期物理量", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"drag_coefficient": 0.32, "lift_coefficient": 1.05, "residual_final": 1e-6, "iterations": 5000}`,
		NodeProgression:    []string{"mesh_sensitivity", "research", "parameter_sweep", "validation"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Setup", Description: "基线仿真 — 验证网格、边界条件、求解器设置\n确认可收敛运行"},
			{Name: "Mesh & Parameter Sensitivity", Description: "网格无关性验证 + 关键参数敏感性分析\n每组配置作为独立节点"},
			{Name: "Core Simulation Campaign", Description: "执行核心研究仿真\n系统变化研究参数，记录物理量"},
			{Name: "Validation & Verification", Description: "与实验数据或解析解对比验证\n不确定性量化分析"},
		},
	},

	// ── molecular: 分子动力学 / DFT / 量子化学 / 药物设计 / 材料科学 ──
	"molecular": {
		Name:               "molecular",
		Languages:          []string{"python", "gaussian"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/trajectory.csv"},
		AcceptCriteria:     []string{"计算正常收敛", "outputs/results.json 存在且包含能量/结构数据", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"total_energy_hartree": -76.03, "binding_energy_kcal": -5.2, "rmsd_angstrom": 0.15, "converged": true}`,
		NodeProgression:    []string{"parameter_study", "production_run", "analysis", "replication"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Structure Optimization", Description: "初始结构优化，验证收敛和稳定性"},
			{Name: "Parameter Study", Description: "测试不同基组/泛函/力场参数\n每组参数作为独立节点"},
			{Name: "Production Run", Description: "执行正式计算/动力学模拟\n记录能量、轨迹等关键数据"},
			{Name: "Analysis & Validation", Description: "数据分析，与实验/文献对比\n复现验证"},
		},
	},

	// ── bioinformatics: 基因组 / 转录组 / 蛋白质 / 单细胞 / 系统生物学 ──
	"bioinformatics": {
		Name:               "bioinformatics",
		Languages:          []string{"python", "R"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/stats_summary.csv"},
		AcceptCriteria:     []string{"分析流程可运行", "outputs/results.json 存在且包含预期统计量", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"auc": 0.87, "p_value": 0.003, "num_significant_genes": 142, "fdr_threshold": 0.05}`,
		NodeProgression:    []string{"preprocessing", "research", "sensitivity_analysis", "validation"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Data Preprocessing & QC", Description: "数据质控、标准化、预处理\n验证输入数据完整性"},
			{Name: "Primary Analysis", Description: "执行核心分析流程\n差异分析、富集分析等"},
			{Name: "Sensitivity & Robustness", Description: "参数敏感性分析\n不同阈值/方法的稳健性检验"},
			{Name: "Cross-validation", Description: "独立数据集验证\n统计显著性检验"},
		},
	},

	// ── statistics: 统计检验 / 因果推断 / 贝叶斯 / 实验设计 ──
	"statistics": {
		Name:               "statistics",
		Languages:          []string{"R", "python"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md"},
		AcceptCriteria:     []string{"分析脚本可运行", "outputs/results.json 存在且包含检验统计量", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"estimate": 2.34, "std_error": 0.41, "p_value": 0.01, "ci_lower": 1.54, "ci_upper": 3.14, "n": 500}`,
		NodeProgression:    []string{"data_exploration", "research", "robustness_check", "replication"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Exploratory Analysis", Description: "描述性统计与数据探索\n检查分布、缺失值、异常值"},
			{Name: "Model Fitting", Description: "核心统计建模与假设检验\n多种模型规格对比"},
			{Name: "Robustness Checks", Description: "敏感性分析、替代模型规格\n不同子样本的稳健性"},
			{Name: "Replication", Description: "在保留样本或模拟数据上复现\n交叉验证或 Bootstrap"},
		},
	},

	// ── econometrics: 计量经济学 / 政策评估 / 社科定量研究 ──
	"econometrics": {
		Name:               "econometrics",
		Languages:          []string{"R", "python", "stata"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/regression_tables.csv"},
		AcceptCriteria:     []string{"分析脚本可运行", "outputs/results.json 存在且包含估计量和检验统计", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"coefficient": 0.45, "std_error": 0.12, "t_stat": 3.75, "p_value": 0.002, "r_squared": 0.68, "n_obs": 10000}`,
		NodeProgression:    []string{"identification", "robustness", "heterogeneity", "replication"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Specification & Identification", Description: "模型设定与识别策略\n定义处理变量、工具变量或断点"},
			{Name: "Core Estimation", Description: "核心回归/匹配/IV 估计\n报告主要系数和标准误"},
			{Name: "Robustness & Heterogeneity", Description: "替代规格检验 + 异质性分析\n不同子样本/带宽/窗口的稳健性"},
			{Name: "Placebo & Replication", Description: "安慰剂检验 + 外部复现\n验证因果关系而非统计伪象"},
		},
	},

	// ── optimization: 运筹学 / 组合优化 / 元启发式 / 调度 ──
	"optimization": {
		Name:               "optimization",
		Languages:          []string{"python"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/convergence.csv"},
		AcceptCriteria:     []string{"优化算法可运行", "outputs/results.json 存在且包含目标函数值", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"objective_value": 1247.3, "best_known": 1210.0, "gap_percent": 3.08, "iterations": 50000, "runtime_sec": 180}`,
		NodeProgression:    []string{"search_strategy", "benchmark", "scaling_study", "replication"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Algorithm", Description: "实现基线算法，验证正确性\n在小规模实例上确认可运行"},
			{Name: "Search Strategy Exploration", Description: "测试不同搜索/优化策略\n每种策略作为独立节点"},
			{Name: "Benchmark Comparison", Description: "在标准测试集上对比\n与已知最优解比较 gap"},
			{Name: "Scaling & Replication", Description: "规模测试 + 多次运行统计\n验证算法可扩展性"},
		},
	},

	// ── systems: 计算机系统 / 网络 / 数据库 / 编译器 / 安全 ──
	"systems": {
		Name:               "systems",
		Languages:          []string{"python", "C++"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/profile.csv"},
		AcceptCriteria:     []string{"系统可正常运行", "outputs/results.json 存在且包含性能数据", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"throughput_ops": 125000, "latency_p50_ms": 2.1, "latency_p99_ms": 15.3, "cpu_percent": 78, "memory_mb": 512}`,
		NodeProgression:    []string{"profiling", "controlled_experiment", "comparison", "scaling"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Profiling", Description: "基线系统性能画像\n记录吞吐、延迟、资源占用"},
			{Name: "Controlled Experiment", Description: "单变量实验，隔离因素影响\n每个变量作为独立节点"},
			{Name: "System Comparison", Description: "与替代方案/前序版本对比\n公平基准测试"},
			{Name: "Scaling Study", Description: "负载/数据量扩展测试\n验证系统可扩展性"},
		},
	},

	// ── engineering: 控制 / 信号处理 / 通信 / 电路 / 机器人 ──
	"engineering": {
		Name:               "engineering",
		Languages:          []string{"python", "matlab"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md"},
		AcceptCriteria:     []string{"处理流程可运行", "outputs/results.json 存在且包含性能指标", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"snr_db": 15.2, "ber": 1.2e-4, "settling_time_ms": 45, "overshoot_percent": 8.3, "stability_margin_db": 6.0}`,
		NodeProgression:    []string{"parameter_sweep", "evaluation", "robustness", "comparison"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Implementation", Description: "实现基线算法/系统，验证功能正确"},
			{Name: "Parameter Sweep", Description: "系统扫描关键参数（SNR、带宽、增益等）\n每组参数作为独立节点"},
			{Name: "Performance Evaluation", Description: "在标准条件/数据集上评估\n记录核心性能指标"},
			{Name: "Robustness & Comparison", Description: "噪声/干扰鲁棒性检验 + 与基准方法对比"},
		},
	},

	// ── climate: 气候 / 地球科学 / 海洋 / 大气 / 遥感分析 ──
	"climate": {
		Name:               "climate",
		Languages:          []string{"python", "fortran"},
		Deliverables:       []string{"outputs/results.json", "outputs/run_report.md", "outputs/timeseries.csv"},
		AcceptCriteria:     []string{"模型可运行且物理量合理", "outputs/results.json 存在", "所有新增文件位于允许目录"},
		MetricArtifactPath: "outputs/results.json",
		ReportArtifactPath: "outputs/run_report.md",
		MetricExample:      `{"rmse": 1.23, "bias": -0.15, "correlation": 0.92, "skill_score": 0.85, "spatial_coverage_percent": 95}`,
		NodeProgression:    []string{"sensitivity", "scenario", "multi_model", "validation"},
		RootNodeType:       "baseline",
		Stages: []config.StageDescription{
			{Name: "Baseline Configuration", Description: "基线模型配置与 spin-up\n验证输出物理量合理"},
			{Name: "Sensitivity Analysis", Description: "关键参数敏感性实验\n每组参数作为独立节点"},
			{Name: "Scenario Runs", Description: "不同情景/强迫条件下的模拟\n对比不同情景的输出差异"},
			{Name: "Validation", Description: "与观测数据/再分析资料对比\n计算技能评分和偏差"},
		},
	},
}

// ResolveDomain returns the DomainProfile for the given domain name.
// If override is non-nil, its non-zero fields override the preset.
// If domain is empty or unknown, returns the "general" preset (domain-neutral default).
func ResolveDomain(domain string, override *config.DomainProfile) config.DomainProfile {
	base, ok := DomainPresets[domain]
	if !ok {
		base = DomainPresets["general"]
	}
	if override == nil {
		return base
	}
	if override.Name != "" {
		base.Name = override.Name
	}
	if len(override.Languages) > 0 {
		base.Languages = override.Languages
	}
	if len(override.Deliverables) > 0 {
		base.Deliverables = override.Deliverables
	}
	if len(override.AcceptCriteria) > 0 {
		base.AcceptCriteria = override.AcceptCriteria
	}
	if override.MetricArtifactPath != "" {
		base.MetricArtifactPath = override.MetricArtifactPath
	}
	if override.ReportArtifactPath != "" {
		base.ReportArtifactPath = override.ReportArtifactPath
	}
	if override.MetricExample != "" {
		base.MetricExample = override.MetricExample
	}
	if len(override.NodeProgression) > 0 {
		base.NodeProgression = override.NodeProgression
	}
	if override.RootNodeType != "" {
		base.RootNodeType = override.RootNodeType
	}
	if len(override.Stages) > 0 {
		base.Stages = override.Stages
	}
	return base
}
