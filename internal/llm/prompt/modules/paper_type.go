package modules

import "github.com/openscholar/openscholar/internal/config"

// NewPaperTypeModule returns a prompt module with type-specific quality criteria.
// Priority 71: loads after tools (70), before cross_reading (72).
func NewPaperTypeModule(paperType config.PaperType) BaseModule {
	rules := PaperTypeRules(paperType)
	if rules == "" {
		return NewBaseModule("paper_type", "", 71)
	}
	return NewBaseModule("paper_type", rules, 71)
}

// PaperTypeRules returns type-specific quality criteria for academic writing.
func PaperTypeRules(paperType config.PaperType) string {
	switch paperType {
	case config.PaperTypeSurvey:
		return `## Survey Paper Quality Criteria
- Minimum 50 references covering the surveyed field comprehensively
- Clear taxonomy or categorization of existing work
- Comparison table covering at least 10 representative methods
- Gap analysis section identifying open problems
- Timeline or evolution narrative showing field progression`

	case config.PaperTypeResearch:
		return `## Research Paper Quality Criteria
- Minimum 20 references with strong baseline coverage
- Clear problem statement and contribution summary
- Complete experimental setup: datasets, metrics, baselines, hyperparameters
- Statistical significance reporting (std dev, confidence intervals)
- Ablation study for each proposed component`

	case config.PaperTypePosition:
		return `## Position Paper Quality Criteria
- Minimum 15 references supporting or contrasting the position
- Clear thesis statement in the introduction
- Structured argumentation with evidence for each claim
- Acknowledgment and rebuttal of counterarguments
- Concrete recommendations or call-to-action`

	case config.PaperTypeThesis:
		return `## Thesis Quality Criteria
- Comprehensive related work chapter (minimum 40 references)
- Clear research questions with hypotheses
- Methodology chapter with reproducibility details
- Results and discussion separated clearly
- Future work section with concrete next steps`

	default:
		return ""
	}
}
