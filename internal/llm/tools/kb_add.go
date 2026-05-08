package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/kb"
)

// supportedDocExtensions are the file types KBAdd can index.
var supportedDocExtensions = map[string]string{
	".pdf":  "pdf",
	".docx": "docx",
	".dotx": "dotx",
	".doc":  "doc",
	".pptx": "pptx",
	".xlsx": "xlsx",
}

type kbAddTool struct {
	kbService kb.Service
	indexer   kb.Indexer
}

type kbAddParams struct {
	FilePath        string `json:"file_path"`
	PDFPath         string `json:"pdf_path,omitempty"` // backward compat
	PaperID         string `json:"paper_id,omitempty"`
	Title           string `json:"title,omitempty"`
	GenerateSummary *bool  `json:"generate_summary,omitempty"`
	SemanticTree    string `json:"semantic_tree,omitempty"` // skip|background|sync
}

// NewKBAddTool creates a tool for adding papers to the knowledge base.
func NewKBAddTool(kbService kb.Service, indexer kb.Indexer) BaseTool {
	return &kbAddTool{kbService: kbService, indexer: indexer}
}

// Available reports whether the KB indexer backend is ready.
func (t *kbAddTool) Available() (bool, string) {
	if t.indexer == nil {
		return false, "python3 or extraction scripts not found"
	}
	return true, ""
}

func (t *kbAddTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "KBAdd",
		Description: "Add a document (PDF, DOCX, PPTX, XLSX) to the knowledge base. Stores raw chunks and index state immediately for search; semantic tree build is optional via semantic_tree mode.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the document file (PDF, DOCX, PPTX, or XLSX)",
				},
				"pdf_path": map[string]any{
					"type":        "string",
					"description": "(Deprecated, use file_path) Absolute path to a PDF file",
				},
				"paper_id": map[string]any{
					"type":        "string",
					"description": "Optional document identifier (e.g., arXiv ID like '2301.12345'). Auto-generated if not provided.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Document title. If not provided, extracted from the document.",
				},
				"generate_summary": map[string]any{
					"type":        "boolean",
					"description": "Whether to generate node summaries (default: true)",
				},
				"semantic_tree": map[string]any{
					"type":        "string",
					"description": "Semantic tree mode: skip (default), background, or sync.",
					"enum":        []string{"skip", "background", "sync"},
				},
			},
			"required": []string{},
		},
	}
}

func (t *kbAddTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params kbAddParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	// Resolve file path: prefer file_path, fall back to pdf_path for backward compat
	filePath := params.FilePath
	if filePath == "" {
		filePath = params.PDFPath
	}
	if filePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
	}

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	docType, ok := supportedDocExtensions[ext]
	if !ok {
		return NewTextErrorResponse(fmt.Sprintf("Unsupported file type: %s. Supported: .pdf, .docx, .dotx, .doc, .pptx, .xlsx", ext)), nil
	}

	rawParser, ok := t.indexer.(kb.RawParser)
	if !ok {
		return NewTextErrorResponse("KB raw parser is not available. Ensure python3 and parsing scripts are installed."), nil
	}

	semanticTreeMode := strings.ToLower(strings.TrimSpace(params.SemanticTree))
	if semanticTreeMode == "" {
		semanticTreeMode = "skip"
	}
	if semanticTreeMode != "skip" && semanticTreeMode != "background" && semanticTreeMode != "sync" {
		return NewTextErrorResponse("semantic_tree must be one of: skip, background, sync"), nil
	}

	parseResult, err := rawParser.ParseDocument(ctx, filePath, kb.ParseOptions{DocType: docType})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to parse document: %v", err)), nil
	}

	// Ensure paper has a valid ID so subsequent tools (KBTree, KBQuery) can reference it.
	paperID := params.PaperID
	if paperID == "" {
		paperID = uuid.New().String()
	}

	paper := kb.Paper{
		PaperID:  paperID,
		Title:    params.Title,
		FilePath: filePath,
		DocType:  docType,
	}
	// Backward compat: also set PDFPath for PDF files
	if docType == "pdf" {
		paper.PDFPath = filePath
	}
	if paper.Title == "" {
		paper.Title = parseResult.PaperTitle
	}

	semanticTreeStatus := "not_requested"
	semanticNote := ""
	semanticTaskID := ""

	stateWriter, ok := t.kbService.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
		AttachSemanticTree(ctx context.Context, paperID string, result kb.IndexResult) error
	})
	if !ok {
		return NewTextErrorResponse("KB service does not support fast ingest APIs"), nil
	}
	state, err := stateWriter.AddParsedPaper(ctx, paper, *parseResult, semanticTreeStatus)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to store document: %v", err)), nil
	}

	if semanticTreeMode == "sync" {
		opts := kb.DefaultIndexOptions()
		if params.GenerateSummary != nil && !*params.GenerateSummary {
			opts.GenerateSummary = false
		}
		if syncRunner, ok := t.kbService.(interface {
			RunSemanticTreeSync(ctx context.Context, indexer kb.Indexer, paperID, filePath string, generateSummary bool) (kb.KBTask, error)
		}); ok {
			task, syncErr := syncRunner.RunSemanticTreeSync(ctx, t.indexer, paper.PaperID, filePath, opts.GenerateSummary)
			semanticTaskID = task.TaskID
			state.SemanticTreeTaskID = task.TaskID
			state.SemanticTreeStatus = task.Status
			if syncErr != nil {
				semanticNote = fmt.Sprintf("Semantic tree sync failed (raw ingest preserved): %v", syncErr)
			} else {
				state.SemanticTreeStatus = "ready"
				state.SemanticTreeAvailable = true
				semanticNote = "Semantic tree synced successfully."
			}
		} else {
			treeResult, buildErr := t.indexer.BuildTree(ctx, filePath, opts)
			if buildErr != nil {
				semanticNote = fmt.Sprintf("Semantic tree sync failed (raw ingest preserved): %v", buildErr)
			} else if attachErr := stateWriter.AttachSemanticTree(ctx, paper.PaperID, *treeResult); attachErr != nil {
				semanticNote = fmt.Sprintf("Semantic tree store failed (raw ingest preserved): %v", attachErr)
			} else {
				state.SemanticTreeStatus = "ready"
				state.SemanticTreeAvailable = true
				semanticNote = "Semantic tree synced successfully."
			}
		}
	}
	if semanticTreeMode == "background" {
		if enqueuer, ok := t.kbService.(interface {
			EnqueueSemanticTreeTask(ctx context.Context, paperID, filePath string, generateSummary bool) (kb.KBTask, error)
		}); ok {
			genSummary := true
			if params.GenerateSummary != nil {
				genSummary = *params.GenerateSummary
			}
			task, enqueueErr := enqueuer.EnqueueSemanticTreeTask(ctx, paper.PaperID, filePath, genSummary)
			if enqueueErr != nil {
				semanticNote = fmt.Sprintf("Semantic tree background enqueue failed (raw ingest preserved): %v", enqueueErr)
			} else {
				semanticTaskID = task.TaskID
				state.SemanticTreeTaskID = task.TaskID
				state.SemanticTreeStatus = task.Status
				semanticTreeStatus = task.Status
				semanticNote = fmt.Sprintf("Semantic tree task queued: %s", task.TaskID)
			}
		} else {
			semanticTreeStatus = "queued_unavailable"
			state.SemanticTreeStatus = semanticTreeStatus
			semanticNote = "Semantic tree mode is background, but durable worker is not available in this phase; raw ingest completed."
		}
	}

	noteBlock := ""
	if semanticNote != "" {
		noteBlock = "\n- " + semanticNote
	}

	resp := NewTextResponse(fmt.Sprintf(
		"Document added to knowledge base and searchable now:\n- ID: %s\n- Title: %s\n- Type: %s\n- Chunks: %d\n- Index level: %s\n- Raw parse status: %s\n- FTS status: %s\n- Semantic tree status: %s\n- Extractor: %s\n- Content available: %t%s\n\nIMPORTANT: Use the ID above (\"%s\") when calling KBTree, KBQuery, or KBSearch for this document.",
		paper.PaperID, paper.Title, docType, len(parseResult.Chunks), state.IndexLevel, state.RawParseStatus, state.FTSStatus, state.SemanticTreeStatus, state.Extractor, state.ContentAvailable, noteBlock, paper.PaperID,
	))
	return WithResponseMetadata(resp, map[string]any{
		"paper_id":                  paper.PaperID,
		"index_level":               state.IndexLevel,
		"raw_parse_status":          state.RawParseStatus,
		"extractor":                 state.Extractor,
		"content_available":         state.ContentAvailable,
		"flat_page_index_available": state.FlatPageIndexAvailable,
		"fts_status":                state.FTSStatus,
		"semantic_tree_status":      state.SemanticTreeStatus,
		"semantic_tree_available":   state.SemanticTreeAvailable,
		"semantic_tree_task_id":     state.SemanticTreeTaskID,
		"total_pages":               state.TotalPages,
		"total_tokens":              state.TotalTokens,
		"content_bytes":             state.ContentBytes,
		"semantic_tree_mode":        semanticTreeMode,
		"semantic_tree_note":        semanticNote,
		"task_id":                   semanticTaskID,
	}), nil
}
