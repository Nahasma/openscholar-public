package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/llm/tools/codeagent"
)

const codeAgentDescription = `Delegate coding tasks to an external AI coding agent (Claude Code, Gemini CLI, Codex, Qwen Code, Trae Agent).
The agent runs autonomously in a subprocess with its own tool set (file read/write, bash, search, etc.) and returns the final result.
Use this tool for complex coding tasks that require iterative development — the external agent can write code, run it, debug errors, and fix them in a loop.

## WHEN TO USE
- Implementing experiment code that requires iterative write-run-debug cycles
- Creating multi-file projects (training pipelines, evaluation harnesses, data processing)
- Setting up project scaffolding with tests and dependencies
- Debugging complex errors that span multiple files
- Reproducing baselines from published papers

## WHEN NOT TO USE
- Simple single-file edits (use Edit tool directly)
- Running a single shell command (use Bash tool directly)
- Reading or searching files (use View/Glob/Grep directly)

## Scientific Research Prompt Templates

Below are research-grade prompt templates. Replace {placeholders} with actual values.
These templates encode best practices from NeurIPS reproducibility guidelines, statistical methodology standards, and ML engineering conventions.

### 1. ML Training Pipeline (PyTorch)
Prompt example:
"Implement a PyTorch training pipeline in code/train.py for {model_name}:
- Dataset: load {dataset_name} from {data_path} using DataLoader (num_workers=4, pin_memory=True)
- Model: {architecture_description} with hyperparameters loaded from config.yaml
- Optimizer: AdamW (lr={lr}, weight_decay={wd}) with cosine annealing schedule (warmup={warmup_steps} steps, T_max={total_steps})
- Training loop: gradient accumulation over {accum_steps} steps (effective batch size = {effective_bs})
- Mixed precision: torch.cuda.amp.GradScaler + autocast for FP16 forward pass
- Reproducibility: set all seeds (torch.manual_seed, np.random.seed, random.seed, torch.backends.cudnn.deterministic=True, torch.backends.cudnn.benchmark=False)
- Checkpointing: save model, optimizer, scheduler, RNG states, and epoch to checkpoints/ every {save_interval} epochs
- Logging: WandB integration logging loss, learning rate, gradient norm, GPU memory, throughput (samples/sec), and wall-clock time
- Evaluation: run validation every {eval_interval} steps, report {val_metrics}
- Early stopping: monitor validation {primary_metric}, patience={patience} epochs
- CLI interface: argparse with all hyperparameters as command-line arguments with defaults from config.yaml
- Requirements: generate requirements.txt with pinned versions"

### 2. Statistical Analysis Script
Prompt example:
"Create a statistical analysis script in code/analysis.py that processes experiment results:
- Load results from {results_csv} (columns: method, seed, {metric_columns})
- For each metric, apply this decision tree:
  1. Shapiro-Wilk normality test (alpha=0.05) on each group
  2. If ALL groups normal: Levene's test for equal variances
     - Equal variance: one-way ANOVA → Tukey HSD post-hoc (if >2 groups) or paired t-test (if 2 groups)
     - Unequal variance: Welch's ANOVA → Games-Howell post-hoc
  3. If ANY group non-normal: Kruskal-Wallis → Dunn's test with Holm-Bonferroni correction
  4. For all pairwise comparisons:
     - Effect size: Cohen's d (parametric) or rank-biserial correlation (non-parametric)
     - 95% CI via BCa bootstrap (n_resamples=10000)
     - Apply Holm-Bonferroni correction for multiple comparisons across all metrics
- Generate LaTeX table: mean±std with significance stars (*p<0.05, **p<0.01, ***p<0.001), best result in bold
- Output table to paper/tables/results.tex in booktabs format
- Log all test statistics, p-values, and effect sizes to results/statistical_tests.json
- NEVER report 'trending toward significance' — either significant at corrected alpha or not"

### 3. Evaluation Harness with Standard Metrics
Prompt example:
"Build an evaluation harness in code/evaluate.py:
- Load model from {checkpoint_path}, set to eval mode with torch.no_grad()
- Run inference on {test_data} with batch_size={batch_size}
- Metric selection by task type:
  * Classification: accuracy, macro-F1, weighted-F1, per-class precision/recall, AUROC (one-vs-rest), confusion matrix
  * NLG (generation): BLEU-4, ROUGE-1/2/L, BERTScore (microsoft/deberta-xlarge-mnli), METEOR, distinct-1/2
  * Information Retrieval: NDCG@{k}, MAP@{k}, MRR, Recall@{k}
  * Regression: MSE, MAE, R², Spearman/Pearson correlation
- For ALL metrics: bootstrap 95% CI (n=1000, stratified by class for classification)
- Report format: mean [95% CI lower, upper], e.g., '84.7 [82.9, 86.5]'
- Ablation protocol: for each component in {ablation_list}, remove ONE component while keeping all others fixed, same seeds/data/hyperparameters, paired bootstrap test vs full model
- Save predictions to results/predictions.jsonl (one JSON object per example with input, gold, predicted, scores)
- Save metrics to results/metrics.json
- Generate LaTeX tables to paper/tables/ in booktabs format
- NEVER report best-of-N-seeds; always report mean across seeds with CI"

### 4. Data Preprocessing Pipeline
Prompt example:
"Create a data preprocessing pipeline in code/preprocess.py:
- Read raw data from {raw_data_path} (format: {csv/json/parquet})
- Data validation gate 1 (schema): verify column names, dtypes, value ranges match expected schema
- Cleaning: handle missing values (strategy: {drop/impute_mean/impute_median/impute_knn}), remove exact duplicates, fix text encoding issues
- Feature engineering: {feature_descriptions}
- Data validation gate 2 (distribution): check for class imbalance, outlier detection (IQR method), verify no constant columns
- Train/val/test split: {train_ratio}/{val_ratio}/{test_ratio} with stratification on {stratify_column}, random_state=42
- Normalization: fit StandardScaler/MinMaxScaler on TRAIN SET ONLY, transform val/test with the same fitted scaler (prevent data leakage)
- Data validation gate 3 (sanity): verify no overlap between splits, check split sizes, validate label distributions match
- Save processed data to data/processed/{train,val,test}.{format}
- Save preprocessing artifacts (scaler, encoder, schema) to data/processed/artifacts/ for inference reproducibility
- Generate data_report.md: shape, dtypes, missing rates, class distribution, feature statistics per split
- Compute SHA-256 hash of raw input file, record in data/processed/manifest.json for provenance"

### 5. Baseline Reproduction
Prompt example:
"Reproduce the baseline from '{paper_title}' (arXiv: {arxiv_id}):
- Reference: Table {table_num}, {method_name} row, reported metrics: {reported_metrics}
- Setup:
  1. If official repo exists ({repo_url}): clone, install dependencies, verify README instructions work
  2. If no official repo: implement from paper description in code/baselines/{method_name}/
- Dataset: {dataset_name} — download from {source}, verify SHA-256 matches {expected_hash}
- Training: use EXACT hyperparameters from paper Section {section} / Appendix {appendix}: {key_hyperparams}
- Run {n_seeds} seeds (42, 43, 44, ...), collect per-seed metrics
- Compare: create comparison table (our reproduction vs reported) with absolute difference and relative % gap
- Document discrepancies: hardware differences, library version mismatches, ambiguous paper descriptions
- Save reproduction log to .handoff/baseline-reproduction.md with: environment spec, exact commands run, per-seed results, comparison table, analysis of any gap >1%
- Output: checkpoint + metrics.json + comparison.tex"

### 6. Visualization (Publication-Quality Plots)
Prompt example:
"Generate publication-quality figures in code/plot.py:
- Style: matplotlib + seaborn, font='serif', fontsize=11pt, figure width={column_width_inches} inches (single-column: 3.25, double: 6.75)
- Color palette: colorblind-safe (seaborn 'colorblind' or custom), consistent across all plots
- Plot 1 — Learning curves: train/val {metric} vs epoch, shaded region = ±1 std across seeds, markers every {marker_interval} epochs
- Plot 2 — Method comparison bar chart: methods on x-axis, {metric} on y-axis, error bars = 95% CI, annotate significant differences with brackets and stars
- Plot 3 — Ablation heatmap: rows = ablation variants, columns = metrics, cell color = relative performance vs full model, annotate with values
- Plot 4 — Representation visualization: t-SNE (perplexity={perp}) or UMAP (n_neighbors={nn}) of {layer_name} embeddings, colored by ground-truth class, with legend
- All plots: tight_layout, no unnecessary chartjunk, axis labels with units, legends outside plot area if >4 entries
- Save each figure as both PDF (vector graphics) and PNG (dpi=300) to paper/figures/
- Generate LaTeX \\includegraphics snippets for each figure in paper/figures/includes.tex
- Use fig.savefig(..., bbox_inches='tight', pad_inches=0.02) for clean bounding boxes"
`

type codeAgentTool struct {
	registry *codeagent.Registry
}

// NewCodeAgentTool creates a CodeAgent tool backed by the given provider registry.
func NewCodeAgentTool(registry *codeagent.Registry) BaseTool {
	return &codeAgentTool{registry: registry}
}

func (t *codeAgentTool) Info() ToolInfo {
	available := t.registry.Available()

	desc := codeAgentDescription
	if len(available) > 0 {
		desc += fmt.Sprintf("\n\nCurrently installed: %s", strings.Join(available, ", "))
	} else {
		desc += "\n\nNo code agent CLI currently installed. Install one of: claude, gemini, codex, qwen-code, trae-cli"
	}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Detailed description of the coding task. Be specific about file paths, requirements, languages, and expected outputs. See the prompt templates in the tool description for research-grade examples.",
			},
			"provider": map[string]any{
				"type":        "string",
				"description": "Which code agent CLI to use. If omitted, auto-selects the best available.",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "Working directory for the code agent. In research mode, defaults to the research workspace. The code agent will read/write files relative to this directory.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Resume a previous coding session. Pass the session_id returned by a prior CodeAgent call to continue where it left off.",
			},
			"max_turns": map[string]any{
				"type":        "integer",
				"description": "Maximum iteration rounds for the code agent (write-run-debug cycles). Default depends on provider (typically 10-20).",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds. Default: 300 (5 minutes). Increase for long-running tasks like training or large codebases.",
			},
		},
		"required": []string{"prompt"},
	}

	// Add dynamic enum for provider if any are available
	if len(available) > 0 {
		params["properties"].(map[string]any)["provider"].(map[string]any)["enum"] = available
	}

	return ToolInfo{
		Name:        "CodeAgent",
		Description: desc,
		Parameters:  params,
		Required:    []string{"prompt"},
	}
}

// Available implements AvailabilityChecker.
func (t *codeAgentTool) Available() (bool, string) {
	available := t.registry.Available()
	if len(available) > 0 {
		return true, ""
	}
	return false, "no code agent CLI installed; install one of: claude, gemini, codex, qwen-code, trae-cli"
}

type codeAgentParams struct {
	Prompt    string `json:"prompt"`
	Provider  string `json:"provider,omitempty"`
	WorkDir   string `json:"workdir,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	MaxTurns  int    `json:"max_turns,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
}

func (t *codeAgentTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params codeAgentParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		// Fallback: try extracting prompt from malformed JSON (BUG-7 pattern)
		prompt := extractJSONStringField(call.Input, "prompt")
		if prompt == "" {
			return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
		}
		params.Prompt = prompt
	}

	if params.Prompt == "" {
		return NewTextErrorResponse("prompt is required — provide a detailed coding task description"), nil
	}

	// Resolve working directory
	workDir := params.WorkDir
	if IsResearchMode(ctx) {
		researchDir := ResearchWorkDir(ctx)
		if workDir == "" {
			workDir = researchDir
		} else if err := ValidateResearchPath(ctx, workDir); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("workdir rejected: %s", err)), nil
		}
	}

	// Get provider
	provider, err := t.registry.Get(params.Provider)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// Apply per-provider config defaults
	maxTurns := params.MaxTurns
	if maxTurns == 0 {
		if cfg, ok := t.registry.GetConfig(provider.Name()); ok && cfg.MaxTurns > 0 {
			maxTurns = cfg.MaxTurns
		}
	}

	timeout := time.Duration(params.Timeout) * time.Second
	if params.Timeout == 0 {
		timeout = 300 * time.Second
	}

	// Execute
	resp, err := provider.Execute(ctx, codeagent.CodeRequest{
		Prompt:    params.Prompt,
		WorkDir:   workDir,
		MaxTurns:  maxTurns,
		SessionID: params.SessionID,
		Timeout:   timeout,
	})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("[%s] execution failed: %s", provider.Name(), err)), nil
	}

	// Build response
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] ", provider.Name())
	if resp.Success {
		sb.WriteString("Task completed.\n\n")
	} else {
		sb.WriteString("Task finished with issues.\n\n")
	}
	sb.WriteString(resp.Result)

	// Include session ID for potential follow-up
	if resp.SessionID != "" {
		fmt.Fprintf(&sb, "\n\n---\nSession ID: %s (pass as session_id to continue this session)", resp.SessionID)
	}

	return NewTextResponse(sb.String()), nil
}
