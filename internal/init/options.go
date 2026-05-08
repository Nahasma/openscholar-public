package initwizard

// SelectOption represents a label-value pair for init wizard options.
type SelectOption struct {
	Label string
	Value string
}

// Exported option sets for use by TUI init wizard.
var (
	DomainOptions = []SelectOption{
		{"Computer Science", "Computer Science"},
		{"Physics", "Physics"},
		{"Mathematics", "Mathematics"},
		{"Chemistry", "Chemistry"},
		{"Biology", "Biology"},
		{"Medicine", "Medicine"},
		{"Engineering", "Engineering"},
		{"Earth & Environmental", "Earth & Environmental"},
		{"Economics", "Economics"},
		{"Social Sciences", "Social Sciences"},
		{"Humanities", "Humanities"},
	}

	LanguageOptions = []SelectOption{
		{"English", "English"},
		{"中文", "Chinese"},
		{"Bilingual (EN+CN)", "Bilingual"},
		{"日本語", "Japanese"},
		{"한국어", "Korean"},
		{"Other", "Other"},
	}

	RoleOptions = []SelectOption{
		{"Undergraduate", "Undergraduate"},
		{"Master", "Master"},
		{"PhD", "PhD"},
		{"Postdoc", "Postdoc"},
		{"Faculty", "Faculty"},
		{"Industry Researcher", "Industry"},
	}

	PubLevelOptions = []SelectOption{
		{"Top-tier (Nature, NeurIPS, Cell, PRL...)", "top"},
		{"Well-known (AAAI, PRB, JACS...)", "known"},
		{"Regular journals", "regular"},
		{"Preprint (arXiv, etc.)", "preprint"},
		{"Thesis / Dissertation", "thesis"},
		{"No specific target yet", "undecided"},
	}

	ResearchTypeOptions = []SelectOption{
		{"Primarily theoretical", "theory"},
		{"Primarily experimental", "experiment"},
		{"Theory + Experiment", "theory+experiment"},
		{"Engineering & Systems", "engineering"},
		{"Survey & Analysis", "survey"},
	}

	ConcurrentOptions = []SelectOption{
		{"1 (focused)", "1"},
		{"2-3", "2-3"},
		{"4+", "4+"},
	}

	LatexOptions = []SelectOption{
		{"Beginner (need syntax guidance)", "beginner"},
		{"Intermediate (can write, occasional issues)", "intermediate"},
		{"Expert (no help needed)", "expert"},
	}

	CollaborationOptions = []SelectOption{
		{"Solo researcher", "solo"},
		{"Small team (2-5 people)", "small_team"},
		{"Large team (cross-group/cross-institution)", "large_team"},
	}

	FeedbackOptions = []SelectOption{
		{"Direct criticism (point out all issues)", "direct"},
		{"Gentle suggestions", "gentle"},
		{"Only comment when asked", "passive"},
	}
)
