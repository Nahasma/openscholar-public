package prompt

// PromptTier defines the loading strategy for prompt modules.
type PromptTier int

const (
	// TierCore includes only the essential modules required for basic operation.
	// Used when PromptTierEnabled is true to reduce token consumption.
	TierCore PromptTier = 0

	// TierExtended includes all modules (full feature set).
	// Used when PromptTierEnabled is false (default behavior).
	TierExtended PromptTier = 1
)

// ExtendedModuleCatalog returns a compact directory of Tier 2 (extended) modules
// with their names and descriptions (~200 tokens). When prompt tier mode is active,
// the agent can request specific modules by name through the skill system.
func ExtendedModuleCatalog() string {
	return `# Extended Prompt Modules (On-Demand)
The following capability modules are available on request. Mention a module name to activate its guidance:

- **kb_workflow**: Autonomous knowledge-base usage patterns and multi-strategy search guidance.
- **tables**: LaTeX table formatting, booktabs style, column alignment, and multirow/multicolumn.
- **outline**: Paper structure planning, section scaffolding, and argument flow design.
- **style**: Academic writing style, tone calibration, and sentence-level polish guidelines.
- **polishing**: Final-pass editing: clarity, concision, transition sentences, and readability.
- **bibtex**: BibTeX entry formatting, key conventions, deduplication, and cite-command usage.
- **symbol_consistency**: Mathematical symbol naming, notation index, and cross-section consistency.
- **visualization**: Figure design, matplotlib/pgfplots code patterns, and caption writing.
- **submission_check**: Venue-specific submission checklist (page limits, anonymization, formatting).
- **compilation**: LaTeX compilation workflow, error diagnosis, and multi-pass build strategy.
- **scholar_search**: Academic search query construction and result evaluation heuristics.
- **doc_export**: Export pipeline for converting LaTeX/Markdown to Word/PDF via pandoc.
- **doc_workflow**: Staged document generation workflow (plan → research → draft → assets → quality gate → export).
- **self_review**: Structured self-review checklist for paper sections before submission.
- **notes**: Reading report quality standards and knowledge-capture conventions.
- **cross_reading**: Cross-paper comparison, gap analysis, and synthesis note patterns.
- **citation_rules**: Citation placement rules, avoid over-citation, and claim attribution.
- **paper_type**: Paper-type-specific writing norms (survey vs. empirical vs. theoretical).
- **cross_compare**: Side-by-side method comparison tables and evaluation matrix design.
- **tikz**: TikZ diagram patterns for neural architectures, flowcharts, and data pipelines.
`
}
