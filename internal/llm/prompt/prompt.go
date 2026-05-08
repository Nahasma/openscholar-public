package prompt

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	initwizard "github.com/Nahasma/openscholar-public/internal/init"
	"github.com/Nahasma/openscholar-public/internal/llm/prompt/modules"
	"github.com/Nahasma/openscholar-public/internal/llm/prompt/modules/agents"
	"github.com/Nahasma/openscholar-public/internal/template"
)

// skillMetaLoader is set by the app layer to provide skill metadata without
// introducing a circular dependency between prompt and skillbank packages.
var skillMetaLoader func() []modules.SkillCatalogEntry

// SetSkillMetaLoader registers the function used to load skill metadata for
// the L1 catalog. Called once during app initialization.
func SetSkillMetaLoader(fn func() []modules.SkillCatalogEntry) {
	skillMetaLoader = fn
}

func loadSkillCatalogEntries() []modules.SkillCatalogEntry {
	if skillMetaLoader == nil {
		return nil
	}
	return skillMetaLoader()
}

func GetAgentPrompt(agentName config.AgentName) string {
	return BuildAgentPromptRuntime(agentName, time.Now()).SystemMessage
}

// BuildAgentPromptBlocks returns cache-aware prompt blocks for a given agent.
// For coder agent, blocks preserve module-level static/dynamic segmentation.
// For other agents, base prompt is returned as one static block.
func BuildAgentPromptBlocks(agentName config.AgentName) []PromptBlock {
	return buildAgentPromptBlocksAt(agentName, time.Now())
}

func buildAgentPromptBlocksAt(agentName config.AgentName, now time.Time) []PromptBlock {
	var blocks []PromptBlock
	switch agentName {
	case config.AgentCoder:
		cwd, _ := os.Getwd()
		blocks = newCoderPromptBuilder(cwd).BuildBlocks()
	default:
		base := getBasePrompt(agentName)
		if base != "" {
			blocks = append(blocks, PromptBlock{
				Text:      base,
				IsDynamic: false,
				CacheKey:  blockCacheKey(base, false),
			})
		}
	}
	if suffix := buildAgentRuntimeSuffix(agentName, now); suffix != "" {
		blocks = append(blocks, PromptBlock{
			Text:      suffix,
			IsDynamic: true,
		})
	}
	return blocks
}

func buildAgentRuntimeSuffix(agentName config.AgentName, now time.Time) string {
	var suffix string
	switch agentName {
	case config.AgentCoder, config.AgentGeneral, config.AgentExplore, config.AgentLeader, config.AgentPlan, config.AgentVerify, config.AgentCoordinator:
		// User custom prompts from prompt.md files
		if custom := LoadCustomPrompts(); custom != "" {
			suffix += "\n\n# User Instructions\n" + custom
		}

		// Environment context
		platform := runtime.GOOS
		shell := os.Getenv("SHELL")
		cwd, _ := os.Getwd()
		suffix += fmt.Sprintf("\n\n# Environment\n- Platform: %s\n- Shell: %s\n- Working directory: %s\n", platform, shell, cwd)
		suffix += FormatRuntimeClockContext(agentName, now)
	case config.AgentSummarizer:
		suffix += FormatRuntimeClockContext(agentName, now)
	}
	return suffix
}

func getBasePrompt(agentName config.AgentName) string {
	switch agentName {
	case config.AgentCoder:
		return buildCoderPrompt()
	case config.AgentGeneral:
		return agents.NewGeneralPromptModule().Content()
	case config.AgentExplore:
		return agents.NewExplorePromptModule().Content()
	case config.AgentPlan:
		return agents.NewPlanPromptModule().Content()
	case config.AgentVerify:
		return agents.NewVerifyPromptModule().Content()
	case config.AgentLeader:
		return modules.NewLeaderModule().Content()
	case config.AgentCoordinator:
		return agents.NewCoordinatorPromptModule().Content()
	case config.AgentTitle:
		return titleSystemPrompt
	case config.AgentSummarizer:
		return summarizerSystemPrompt
	default:
		return buildCoderPrompt()
	}
}

// buildCoderPrompt assembles the Coder Agent system prompt from modules.
// When config.Harness.PromptTierEnabled is true, only core modules are injected
// and an extended module catalog is appended (TierCore mode).
// Otherwise all modules are injected as usual (TierExtended / legacy mode).
func buildCoderPrompt() string {
	cwd, _ := os.Getwd()
	return newCoderPromptBuilder(cwd).Build()
}

func newCoderPromptBuilder(cwd string) *PromptBuilder {
	builder := NewPromptBuilder()
	builder.Add(modules.NewBasePromptModule()) // priority 0

	// User profile from Soft Init (priority 1) — always injected (dynamic)
	if pd := loadProfileForPrompt(cwd); pd != nil {
		builder.Add(modules.NewProfileModule(*pd))
	}

	// Deferred tools catalog — always included
	builder.Add(modules.NewDeferredToolsModule([]modules.DeferredToolEntry{
		{Name: "ImageGen", Description: "Generate AI images for research figures (GLM CogView, MiniMax, DALL-E)."},
		{Name: "DiagramGen", Description: "Generate D2 diagrams for architecture and flow charts."},
		{Name: "PaperValidate", Description: "Validate paper quality (placeholders, citations, figures)."},
		{Name: "ScholarSearch", Description: "Search academic papers; for broad literature scans prefer Task agent_type=research so raw hits stay isolated."},
		{Name: "KBAdd", Description: "Fast-ingest a document into the knowledge base as searchable raw chunks; semantic tree is optional."},
		{Name: "DocExport", Description: "Convert documents between LaTeX, Markdown, Word, and PDF formats. Auto-selects PDF engine with CJK font support."},
		{Name: "DocQualityGate", Description: "Check Markdown document quality before export: empty sections, broken images, placeholders, missing promised assets."},
		{Name: "KBQuery", Description: "Ask questions about a specific paper already in the user's local knowledge base.", Domain: "kb", CostTier: "high", Intent: "local_kb", Scope: "turn"},
		{Name: "KBSearch", Description: "Search and synthesize answers from papers in the user's local knowledge base.", Domain: "kb", CostTier: "high", Intent: "local_kb", Scope: "turn"},
		{Name: "KBList", Description: "List papers already ingested in the user's local knowledge base.", Domain: "kb", CostTier: "high", Intent: "local_kb", Scope: "turn"},
		{Name: "KBTree", Description: "Show semantic tree, flat page index, or indexing status for a KB paper."},
		{Name: "Task", Description: "Spawn a sub-agent for parallel or isolated tasks; use agent_type=research for broad ScholarSearch/WebSearch fanout."},
		{Name: "AskUser", Description: "Ask the user for clarification on ambiguous instructions."},
		{Name: "SkillQuery", Description: "Search skill metadata and view full skill instructions by id."},
		{Name: "SkillManage", Description: "Create, update, delete, and govern skills after explicit user confirmation."},
		{Name: "RecordFeedback", Description: "Record user feedback on Agent output for skill evolution."},
	})) // priority 6

	// Skill catalog (L1 discovery layer) — always injected so the agent knows
	// which skills are available and can call SkillQuery to load full instructions.
	if entries := loadSkillCatalogEntries(); len(entries) > 0 {
		builder.Add(modules.NewSkillCatalogModule(entries, 200000))
	}
	builder.Add(modules.NewSubagentOrchestrationModule(config.Get() != nil && config.Get().SubagentOrchestration.Enabled)) // priority 74
	builder.Add(modules.NewFeedbackCollectorModule())                                                                      // priority 8
	builder.Add(modules.NewSkillOpportunityModule())                                                                       // priority 9

	tierEnabled := config.Get() != nil && config.Get().Harness.PromptTierEnabled

	if tierEnabled {
		// TierCore: inject only essential modules + extended module catalog
		builder.Add(modules.NewToolsModule())   // priority 5
		builder.Add(modules.NewLatexModule())   // priority 10
		builder.Add(modules.NewErrorsModule())  // priority 25
		builder.Add(modules.NewAskUserModule()) // priority 15

		// Extended module catalog so the LLM knows what else is available
		builder.Add(modules.NewBaseModule("extended_catalog", ExtendedModuleCatalog(), 999))
	} else {
		// TierExtended (default): inject all modules
		builder.Add(modules.NewKBWorkflowModule())        // priority 2
		builder.Add(modules.NewToolsModule())             // priority 5
		builder.Add(modules.NewLatexModule())             // priority 10
		builder.Add(modules.NewTablesModule())            // priority 20
		builder.Add(modules.NewErrorsModule())            // priority 25
		builder.Add(modules.NewOutlineModule())           // priority 30
		builder.Add(modules.NewStyleModule())             // priority 35
		builder.Add(modules.NewPolishingModule())         // priority 45
		builder.Add(modules.NewBibtexModule())            // priority 50
		builder.Add(modules.NewSymbolConsistencyModule()) // priority 55
		builder.Add(modules.NewVisualizationModule())     // priority 37
		builder.Add(modules.NewSubmissionCheckModule())   // priority 60
		builder.Add(modules.NewCompilationModule())       // priority 62
		builder.Add(modules.NewScholarSearchModule())     // priority 65
		builder.Add(modules.NewDocExportModule())         // priority 63
		builder.Add(modules.NewDocWorkflowModule())       // priority 64
		builder.Add(modules.NewSelfReviewModule())        // priority 68
		builder.Add(modules.NewNotesModule())             // priority 70
		builder.Add(modules.NewCrossReadingModule())      // priority 72
		builder.Add(modules.NewAskUserModule())           // priority 15
		builder.Add(modules.NewCitationRulesModule())     // priority 5
	}

	// Dynamic conference module based on detected template — always injected
	if cfg := template.Detect(cwd); cfg != nil {
		if cm := modules.NewConferenceModule(cfg); cm != nil {
			builder.Add(cm)
		}
	}

	return builder
}

// loadProfileForPrompt reads the user profile and converts to ProfileData for prompt rendering.
func loadProfileForPrompt(workingDir string) *modules.ProfileData {
	profile, err := initwizard.LoadProfile(workingDir)
	if err != nil || profile == nil {
		return nil
	}

	return &modules.ProfileData{
		Domain:        profile.Domain,
		Subdomain:     profile.Subdomain,
		Language:      profile.Language,
		Role:          profile.Role,
		PubLevel:      profile.PubLevelDisplay(),
		ResearchType:  profile.ResearchType,
		Weaknesses:    profile.FormatWeaknesses(),
		FeedbackStyle: profile.FeedbackStyle,
		SpecialNeeds:  profile.SpecialNeeds,
		LaTeXLevel:    profile.LaTeXLevel,
		Collaboration: profile.Collaboration,
	}
}
