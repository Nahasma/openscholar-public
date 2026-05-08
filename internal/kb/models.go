package kb

// Paper represents a paper or document in the knowledge base.
type Paper struct {
	PaperID   string   `json:"paper_id"`
	Title     string   `json:"title"`
	Authors   []Author `json:"authors,omitempty"`
	Year      int      `json:"year,omitempty"`
	Venue     string   `json:"venue,omitempty"`
	Abstract  string   `json:"abstract,omitempty"`
	DOI       string   `json:"doi,omitempty"`
	ArxivID   string   `json:"arxiv_id,omitempty"`
	PDFPath   string   `json:"pdf_path,omitempty"`
	FilePath  string   `json:"file_path,omitempty"` // General file path (PDF, DOCX, PPTX, XLSX)
	DocType   string   `json:"doc_type,omitempty"`  // "pdf", "docx", "pptx", "xlsx"
	IndexedAt int64    `json:"indexed_at,omitempty"`
}

// Author represents a paper author.
type Author struct {
	Name string `json:"name"`
}

// TreeNode represents a node in the PageIndex tree.
type TreeNode struct {
	NodeID     string     `json:"node_id"`
	Title      string     `json:"title"`
	StartIndex int        `json:"start_index"`
	EndIndex   int        `json:"end_index"`
	Summary    string     `json:"summary,omitempty"`
	Content    string     `json:"content,omitempty"`
	Nodes      []TreeNode `json:"nodes,omitempty"`
}

// PaperTree represents the complete PageIndex tree for a paper.
type PaperTree struct {
	DocName        string     `json:"doc_name"`
	DocDescription string     `json:"doc_description,omitempty"`
	Structure      []TreeNode `json:"structure"`
}

// NodeSummary is a flattened node record for DB storage.
type NodeSummary struct {
	PaperID   string `json:"paper_id"`
	NodeID    string `json:"node_id"`
	Title     string `json:"title"`
	StartPage int    `json:"start_page"`
	EndPage   int    `json:"end_page"`
	Summary   string `json:"summary"`
	Content   string `json:"content,omitempty"`
}

// NodeContent is the full text stored for a flattened tree node.
type NodeContent struct {
	PaperID    string `json:"paper_id"`
	NodeID     string `json:"node_id"`
	Title      string `json:"title,omitempty"`
	Content    string `json:"content"`
	TokenCount int    `json:"token_count,omitempty"`
	Source     string `json:"source,omitempty"`
}

// PaperChunk is the raw-first content unit stored for a paper.
type PaperChunk struct {
	PaperID    string `json:"paper_id"`
	ChunkID    string `json:"chunk_id"`
	Kind       string `json:"kind"`
	PageStart  int    `json:"page_start,omitempty"`
	PageEnd    int    `json:"page_end,omitempty"`
	Title      string `json:"title,omitempty"`
	Content    string `json:"content,omitempty"`
	TokenCount int    `json:"token_count,omitempty"`
	Source     string `json:"source,omitempty"`
	CreatedAt  int64  `json:"created_at,omitempty"`
}

// ParseOptions configures raw parser behavior.
type ParseOptions struct {
	DocType        string `json:"doc_type,omitempty"`
	Parser         string `json:"parser,omitempty"`
	MaxPages       int    `json:"max_pages,omitempty"`
	MaxChunkTokens int    `json:"max_chunk_tokens,omitempty"`
}

// ParseResult is the normalized output from a raw parser.
type ParseResult struct {
	PaperTitle   string       `json:"paper_title,omitempty"`
	DocType      string       `json:"doc_type,omitempty"`
	Chunks       []PaperChunk `json:"chunks,omitempty"`
	TotalPages   int          `json:"total_pages,omitempty"`
	TotalTokens  int          `json:"total_tokens,omitempty"`
	ContentBytes int          `json:"content_bytes,omitempty"`
	Extractor    string       `json:"extractor,omitempty"`
	Warnings     []string     `json:"warnings,omitempty"`
}

// SemanticTreeOptions controls semantic tree build behavior.
type SemanticTreeOptions struct {
	Model           string `json:"model,omitempty"`
	GenerateSummary bool   `json:"generate_summary,omitempty"`
}

// SemanticTreeResult holds semantic tree build output and status.
type SemanticTreeResult struct {
	Tree      PaperTree `json:"tree"`
	ModelUsed string    `json:"model_used,omitempty"`
	Warnings  []string  `json:"warnings,omitempty"`
}

// QueryOptions controls route selection and context shaping for QueryPaper.
type QueryOptions struct {
	PreferredRoute  string   `json:"preferred_route,omitempty"`
	TopKChunks      int      `json:"top_k_chunks,omitempty"`
	MaxContextChars int      `json:"max_context_chars,omitempty"`
	CandidateHints  []string `json:"candidate_hints,omitempty"`
	AllowTaskCreate bool     `json:"allow_task_create,omitempty"`
}

// SearchResult contains the answer to a tree search query.
type SearchResult struct {
	Answer      string            `json:"answer"`
	Sources     []SourceRef       `json:"sources"`
	Thinking    string            `json:"thinking"`
	Diagnostics SearchDiagnostics `json:"diagnostics,omitempty"`
}

// CrossSearchDiagnostics captures structured retrieval state for cross-paper search.
type CrossSearchDiagnostics struct {
	Query               string                  `json:"query,omitempty"`
	FTSQuery            string                  `json:"fts_query,omitempty"`
	SanitizedQuery      string                  `json:"sanitized_query,omitempty"`
	Channels            []SearchChannelHit      `json:"channels,omitempty"`
	Papers              []CrossPaperDiagnostics `json:"papers,omitempty"`
	CandidatePaperCount int                     `json:"candidate_paper_count,omitempty"`
	CandidateChunkCount int                     `json:"candidate_chunk_count,omitempty"`
	SourceCount         int                     `json:"source_count,omitempty"`
	FinalRoute          string                  `json:"final_route,omitempty"`
	LLMCalled           bool                    `json:"llm_called,omitempty"`
	LLMCallCount        int                     `json:"llm_call_count,omitempty"`
	NoHitReason         string                  `json:"no_hit_reason,omitempty"`
	FallbackReason      string                  `json:"fallback_reason,omitempty"`
	Warnings            []string                `json:"warnings,omitempty"`
	Errors              []string                `json:"errors,omitempty"`
}

// SearchChannelHit describes one retrieval channel used by a query.
type SearchChannelHit struct {
	Channel        string   `json:"channel"`
	FTSQuery       string   `json:"fts_query,omitempty"`
	HitCount       int      `json:"hit_count"`
	CandidateCount int      `json:"candidate_count,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	Errors         []string `json:"errors,omitempty"`
}

// CrossPaperDiagnostics describes why a paper was or was not used.
type CrossPaperDiagnostics struct {
	PaperID             string   `json:"paper_id"`
	PaperTitle          string   `json:"paper_title,omitempty"`
	HitChannels         []string `json:"hit_channels,omitempty"`
	FTSQuery            string   `json:"fts_query,omitempty"`
	CandidateChunkCount int      `json:"candidate_chunk_count,omitempty"`
	FinalRoute          string   `json:"final_route,omitempty"`
	LLMCalled           bool     `json:"llm_called,omitempty"`
	LLMCallCount        int      `json:"llm_call_count,omitempty"`
	SourceCount         int      `json:"source_count,omitempty"`
	NoHitReason         string   `json:"no_hit_reason,omitempty"`
	FallbackReason      string   `json:"fallback_reason,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
	Errors              []string `json:"errors,omitempty"`
}

const (
	IndexLevelFullTree       = "full_tree"
	IndexLevelSimpleFullText = "simple_full_text"
	IndexLevelSummaryOnly    = "summary_only"
	IndexLevelUnknown        = "unknown"

	RetrievalModeFullContent     = "full_content"
	RetrievalModeTreeSummary     = "tree_summary"
	RetrievalModeSummaryOnly     = "summary_only"
	RetrievalModeLexicalFallback = "lexical_fallback"
)

// SearchDiagnostics captures bounded, non-sensitive details about KB retrieval.
type SearchDiagnostics struct {
	Route                  string   `json:"route,omitempty"`
	IndexLevel             string   `json:"index_level,omitempty"`
	RetrievalMode          string   `json:"retrieval_mode,omitempty"`
	RawParseStatus         string   `json:"raw_parse_status,omitempty"`
	ParseRetryCount        int      `json:"parse_retry_count,omitempty"`
	SelectionParseError    string   `json:"selection_parse_error,omitempty"`
	SelectionSnippet       string   `json:"selection_snippet,omitempty"`
	ChunkHitCount          int      `json:"chunk_hit_count,omitempty"`
	CandidateChunkCount    int      `json:"candidate_chunk_count,omitempty"`
	SelectedChunkIDs       []string `json:"selected_chunk_ids,omitempty"`
	SelectedNodeIDs        []string `json:"selected_node_ids,omitempty"`
	InvalidNodeIDs         []string `json:"invalid_node_ids,omitempty"`
	TreeSelectMS           int64    `json:"tree_select_ms,omitempty"`
	AnswerMS               int64    `json:"answer_ms,omitempty"`
	LLMCallCount           int      `json:"llm_call_count,omitempty"`
	LLMCalled              bool     `json:"llm_called,omitempty"`
	FallbackUsed           bool     `json:"fallback_used,omitempty"`
	FallbackReason         string   `json:"fallback_reason,omitempty"`
	ContentAvailable       bool     `json:"content_available"`
	FTSStatus              string   `json:"fts_status,omitempty"`
	FTSQuery               string   `json:"fts_query,omitempty"`
	SemanticTreeStatus     string   `json:"semantic_tree_status,omitempty"`
	SemanticTreeAvailable  bool     `json:"semantic_tree_available,omitempty"`
	FlatPageIndexAvailable bool     `json:"flat_page_index_available,omitempty"`
	SanitizedQuery         string   `json:"sanitized_query,omitempty"`
	TaskID                 string   `json:"task_id,omitempty"`
	DeepReadRecommended    bool     `json:"deep_read_recommended,omitempty"`
	UsedFullContent        bool     `json:"used_full_content,omitempty"`
	ContentSearchError     string   `json:"content_search_error,omitempty"`
	ContentChars           int      `json:"content_chars,omitempty"`
	ContentTruncated       bool     `json:"content_truncated,omitempty"`
	Warning                string   `json:"warning,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
	Errors                 []string `json:"errors,omitempty"`
	NoHitReason            string   `json:"no_hit_reason,omitempty"`
	SourceCount            int      `json:"source_count,omitempty"`
	TaskStatus             string   `json:"task_status,omitempty"`
}

// IndexMetadata describes the fidelity of a stored KB index.
type IndexMetadata struct {
	IndexLevel          string           `json:"index_level"`
	Extractor           string           `json:"extractor,omitempty"`
	FallbackReason      string           `json:"fallback_reason,omitempty"`
	ContentAvailable    bool             `json:"content_available"`
	TreeAvailable       bool             `json:"tree_available"`
	SummaryOnly         bool             `json:"summary_only"`
	MissingCapabilities []string         `json:"missing_capabilities,omitempty"`
	Health              *PageIndexHealth `json:"health,omitempty"`
	ModelUsed           string           `json:"model_used,omitempty"`
	TotalPages          int              `json:"total_pages,omitempty"`
	TotalTokens         int              `json:"total_tokens,omitempty"`
	ContentBytes        int              `json:"content_bytes,omitempty"`
}

// PaperIndexState stores durable indexing capability status.
type PaperIndexState struct {
	PaperID                string `json:"paper_id"`
	IndexLevel             string `json:"index_level,omitempty"`
	RawParseStatus         string `json:"raw_parse_status,omitempty"`
	ContentAvailable       bool   `json:"content_available"`
	ContentBytes           int    `json:"content_bytes,omitempty"`
	FlatPageIndexAvailable bool   `json:"flat_page_index_available,omitempty"`
	FTSStatus              string `json:"fts_status,omitempty"`
	SemanticTreeStatus     string `json:"semantic_tree_status,omitempty"`
	SemanticTreeAvailable  bool   `json:"semantic_tree_available,omitempty"`
	SemanticTreeError      string `json:"semantic_tree_error,omitempty"`
	SemanticTreeTaskID     string `json:"semantic_tree_task_id,omitempty"`
	Extractor              string `json:"extractor,omitempty"`
	FallbackReason         string `json:"fallback_reason,omitempty"`
	TotalPages             int    `json:"total_pages,omitempty"`
	TotalTokens            int    `json:"total_tokens,omitempty"`
	SourceFileAvailable    bool   `json:"source_file_available,omitempty"`
	FileHash               string `json:"file_hash,omitempty"`
	UpdatedAt              int64  `json:"updated_at,omitempty"`
}

// KBTask is a durable background task record.
type KBTask struct {
	TaskID       string  `json:"task_id"`
	PaperID      string  `json:"paper_id,omitempty"`
	TaskType     string  `json:"task_type"`
	Status       string  `json:"status"`
	Progress     float64 `json:"progress,omitempty"`
	InputJSON    string  `json:"input_json,omitempty"`
	ResultJSON   string  `json:"result_json,omitempty"`
	ResultRef    string  `json:"result_ref,omitempty"`
	Error        string  `json:"error,omitempty"`
	LeaseOwner   string  `json:"lease_owner,omitempty"`
	LeaseUntil   int64   `json:"lease_until,omitempty"`
	AttemptCount int     `json:"attempt_count,omitempty"`
	CreatedAt    int64   `json:"created_at,omitempty"`
	UpdatedAt    int64   `json:"updated_at,omitempty"`
	StartedAt    int64   `json:"started_at,omitempty"`
	FinishedAt   int64   `json:"finished_at,omitempty"`
}

const (
	KBTaskStatusQueued    = "queued"
	KBTaskStatusRunning   = "running"
	KBTaskStatusSucceeded = "succeeded"
	KBTaskStatusFailed    = "failed"
	KBTaskStatusCanceled  = "canceled"

	KBTaskTypeSemanticTree = "semantic_tree_build"
	KBTaskTypeDeepRead     = "deep_read"
	KBTaskTypeRepair       = "repair"
	KBTaskTypeReindex      = "reindex"
)

// SourceRef identifies the source of a piece of information.
type SourceRef struct {
	NodeID    string `json:"node_id"`
	Title     string `json:"title"`
	StartPage int    `json:"start_page"`
	EndPage   int    `json:"end_page"`
}

// IndexOptions configures the tree building process.
type IndexOptions struct {
	Model               string // LLM model for tree generation
	GenerateSummary     bool
	GenerateDescription bool
	MaxPagesPerNode     int
	MaxTokensPerNode    int
	PDFParser           string // "PyMuPDF" (default) or "PyPDF2"
}

// DefaultIndexOptions returns the default indexing configuration.
func DefaultIndexOptions() IndexOptions {
	return IndexOptions{
		GenerateSummary:     true,
		GenerateDescription: true,
		MaxPagesPerNode:     10,
		MaxTokensPerNode:    20000,
		PDFParser:           "PyMuPDF",
	}
}

// IndexResult contains the output of the indexing process.
type IndexResult struct {
	Tree                PaperTree
	TotalPages          int
	TotalTokens         int
	ModelUsed           string
	IndexLevel          string
	Extractor           string
	FallbackReason      string
	ContentAvailable    bool
	ContentBytes        int
	TreeAvailable       bool
	SummaryOnly         bool
	MissingCapabilities []string
	Health              *PageIndexHealth
}
