package kb

import "context"

// PaperStore handles CRUD operations for papers.
type PaperStore interface {
	AddPaper(ctx context.Context, paper Paper, tree PaperTree) error
	GetPaper(ctx context.Context, paperID string) (*Paper, error)
	ListPapers(ctx context.Context, limit, offset int) ([]Paper, error)
	RemovePaper(ctx context.Context, paperID string) error
	CountPapers(ctx context.Context) (int64, error)
}

// PaperSearch handles paper lookup and search operations.
type PaperSearch interface {
	SearchByTitle(ctx context.Context, query string, limit int) ([]Paper, error)
	FindByDOI(ctx context.Context, doi string) (*Paper, error)
	FindByArxivID(ctx context.Context, arxivID string) (*Paper, error)
	GetNodeSummaries(ctx context.Context, paperID string) ([]NodeSummary, error)
	GetTree(ctx context.Context, paperID string) (*PaperTree, error)
}

// PaperAnalyzer handles cross-paper analysis.
type PaperAnalyzer interface {
	CrossPaperSearch(ctx context.Context, callLLM LLMCaller, question string, limit int) (*CrossSearchResult, error)
}

// RawParser parses documents into raw-first chunks.
type RawParser interface {
	ParseDocument(ctx context.Context, filePath string, opts ParseOptions) (*ParseResult, error)
}

// SemanticTreeBuilder builds semantic trees from parsed chunks.
type SemanticTreeBuilder interface {
	BuildSemanticTree(ctx context.Context, paperID string, chunks []PaperChunk, opts SemanticTreeOptions) (*SemanticTreeResult, error)
}

// PaperQuerier routes and answers single-paper questions.
type PaperQuerier interface {
	QueryPaper(ctx context.Context, callLLM LLMCaller, paperID string, question string, opts QueryOptions) (*SearchResult, error)
}
