package kb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/docx"
	"github.com/openscholar/openscholar/internal/llm/models"
)

// PageIndexError represents a structured error from the PageIndex Python wrapper.
type PageIndexError struct {
	Type      string `json:"error_type"`
	Message   string `json:"error_message"`
	Retryable bool   `json:"retryable"`
}

func (e *PageIndexError) Error() string {
	return fmt.Sprintf("pageindex %s: %s", e.Type, e.Message)
}

type pythonIndexer struct {
	pythonPath          string
	scriptPath          string // PageIndex script for PDFs
	simpleExtractScript string // simple PyMuPDF extract script (fallback)
	docparseScript      string // docparse script for DOCX/PPTX/XLSX (may be empty)
	mineruAvail         bool   // whether MinerU CLI is available
	pageindexAvail      bool   // whether PageIndex script is available
}

// documentExtensions maps file extensions to the docparse script.
var documentExtensions = map[string]bool{
	".docx": true,
	".dotx": true,
	".doc":  true,
	".pptx": true,
	".xlsx": true,
}

// NewPythonIndexer creates an Indexer that calls the PageIndex Python script.
// If PageIndex is unavailable, it falls back to simple PyMuPDF text extraction.
// Only returns error if python3 itself is not found.
func NewPythonIndexer() (Indexer, error) {
	pythonPath := config.PythonPath()
	if pythonPath == "" {
		return nil, fmt.Errorf("python3 not found in PATH")
	}

	scriptPath := findScript("scripts/run_pageindex.py")
	simpleExtractScript := findScript("scripts/run_simple_extract.py")

	// docparse script is optional
	docparseScript := findScript("scripts/run_docparse.py")

	// Even without PageIndex, we can still do simple extraction
	if scriptPath == "" && simpleExtractScript == "" {
		return nil, fmt.Errorf("no PDF extraction scripts found")
	}

	return &pythonIndexer{
		pythonPath:          pythonPath,
		scriptPath:          scriptPath,
		simpleExtractScript: simpleExtractScript,
		docparseScript:      docparseScript,
		mineruAvail:         hasMinerU(),
		pageindexAvail:      scriptPath != "",
	}, nil
}

func (p *pythonIndexer) BuildTree(ctx context.Context, filePath string, opts IndexOptions) (*IndexResult, error) {
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	// .docx/.dotx → Go-native engine first, fallback to Python
	if ext == ".docx" || ext == ".dotx" {
		result, err := buildDocTreeNative(filePath)
		if err == nil {
			return result, nil
		}
		log.Printf("Native DOCX indexer failed, falling back to Python: %v", err)
		return p.buildDocTree(ctx, filePath)
	}

	// .doc → normalize + native, fallback to Python
	if ext == ".doc" {
		if docx.IsNormalizationAvailable() {
			if docxPath, err := docx.NormalizeOfficeInput(ctx, filePath); err == nil {
				if result, err := buildDocTreeNative(docxPath); err == nil {
					return result, nil
				}
			}
		}
		// Fallback to Python
		if p.docparseScript != "" {
			return p.buildDocTree(ctx, filePath)
		}
		return nil, fmt.Errorf("cannot index .doc file: no converter or Python parser available")
	}

	if documentExtensions[ext] {
		return p.buildDocTree(ctx, filePath)
	}
	return p.buildPDFTree(ctx, filePath, opts)
}

// buildPDFTree uses a three-level fallback chain for PDF indexing:
//
//	Level 1: MinerU (high-accuracy OCR + formula + table extraction)
//	Level 2: PageIndex (PyMuPDF + LLM tree structuring)
//	Level 3: Simple PyMuPDF text extraction (flat per-page nodes, no LLM)
func (p *pythonIndexer) buildPDFTree(ctx context.Context, pdfPath string, opts IndexOptions) (*IndexResult, error) {
	var fallbackReasons []string
	// Level 1: MinerU
	if p.mineruAvail {
		result, err := p.buildPDFTreeWithMinerU(ctx, pdfPath, opts)
		if err == nil {
			return finalizeIndexResult(result, IndexLevelFullTree, "mineru", strings.Join(fallbackReasons, "; "), nil), nil
		}
		fallbackReasons = append(fallbackReasons, "MinerU failed: "+err.Error())
		log.Printf("MinerU failed, falling back to PageIndex: %v", err)
	}

	// Level 2: PageIndex
	if p.pageindexAvail {
		result, err := p.buildPDFTreeWithPageIndex(ctx, pdfPath, opts)
		if err == nil {
			return finalizeIndexResult(result, IndexLevelFullTree, "pageindex", strings.Join(fallbackReasons, "; "), nil), nil
		}
		fallbackReasons = append(fallbackReasons, "PageIndex failed: "+err.Error())
		log.Printf("PageIndex failed, falling back to simple extraction: %v", err)
	}

	// Level 3: Simple PyMuPDF extraction
	result, err := p.buildPDFTreeSimple(ctx, pdfPath)
	if err != nil {
		return nil, err
	}
	return finalizeIndexResult(result, "", "simple_extract", strings.Join(fallbackReasons, "; "), nil), nil
}

// buildPDFTreeWithPageIndex uses the PageIndex Python script (PyMuPDF-based).
func (p *pythonIndexer) buildPDFTreeWithPageIndex(ctx context.Context, pdfPath string, opts IndexOptions) (*IndexResult, error) {
	// Preflight: check Python environment health before invoking PageIndex
	health := CheckPageIndexHealth(p.pythonPath)
	if !health.Available {
		return nil, fmt.Errorf("PageIndex unavailable: openai package not installed (run: pip install openai)")
	}
	if !health.SSLOk && health.SSLVersion != "" {
		return nil, fmt.Errorf("PageIndex requires OpenSSL >= 1.1.1, found: %s", health.SSLVersion)
	}
	if !health.OpenAIKeyOk {
		return nil, fmt.Errorf("PageIndex requires an OpenAI-compatible API key; configure one in settings")
	}

	args := []string{
		p.scriptPath,
		"--pdf_path", pdfPath,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.GenerateSummary {
		args = append(args, "--generate_summary")
	}
	if opts.GenerateDescription {
		args = append(args, "--generate_description")
	}
	if opts.MaxPagesPerNode > 0 {
		args = append(args, "--max_pages_per_node", fmt.Sprintf("%d", opts.MaxPagesPerNode))
	}
	if opts.PDFParser != "" {
		args = append(args, "--pdf_parser", opts.PDFParser)
	}

	result, err := p.runScript(ctx, args)
	if err != nil {
		return nil, err
	}
	return finalizeIndexResult(result, IndexLevelFullTree, "pageindex", "", &health), nil
}

// buildPDFTreeWithMinerU uses MinerU CLI for high-accuracy PDF parsing
// with OCR, formula, and table extraction.
func (p *pythonIndexer) buildPDFTreeWithMinerU(ctx context.Context, pdfPath string, opts IndexOptions) (*IndexResult, error) {
	outDir, err := os.MkdirTemp("", "mineru-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(outDir)

	cmd := exec.CommandContext(ctx, "mineru", "-p", pdfPath, "-o", outDir, "-m", "auto")
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mineru command failed: %s", string(output))
	}

	// MinerU outputs Markdown files; find the first one
	mdFiles, _ := filepath.Glob(filepath.Join(outDir, "**", "*.md"))
	if len(mdFiles) == 0 {
		mdFiles, _ = filepath.Glob(filepath.Join(outDir, "*.md"))
	}
	if len(mdFiles) == 0 {
		return nil, fmt.Errorf("MinerU produced no markdown output")
	}

	// Pass MinerU Markdown to PageIndex for tree structuring
	args := []string{
		p.scriptPath,
		"--markdown_path", mdFiles[0],
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.GenerateSummary {
		args = append(args, "--generate_summary")
	}
	if opts.GenerateDescription {
		args = append(args, "--generate_description")
	}

	result, err := p.runScript(ctx, args)
	if err != nil {
		return nil, err
	}
	return finalizeIndexResult(result, IndexLevelFullTree, "mineru", "", nil), nil
}

// buildPDFTreeSimple uses PyMuPDF to extract text per-page without tree structuring.
// This is the lowest-fidelity fallback: no LLM calls, no hierarchy, just raw text per page.
func (p *pythonIndexer) buildPDFTreeSimple(ctx context.Context, pdfPath string) (*IndexResult, error) {
	if p.simpleExtractScript == "" {
		return nil, fmt.Errorf("simple extract script not found")
	}

	args := []string{
		p.simpleExtractScript,
		"--pdf_path", pdfPath,
	}
	result, err := p.runScript(ctx, args)
	if err != nil {
		return nil, err
	}
	return finalizeIndexResult(result, "", "simple_extract", "", nil), nil
}

// hasMinerU checks if MinerU CLI is available in PATH.
func hasMinerU() bool {
	_, err := exec.LookPath("mineru")
	return err == nil
}

// buildDocTree calls the docparse Python script for DOCX/PPTX/XLSX files.
func (p *pythonIndexer) buildDocTree(ctx context.Context, filePath string) (*IndexResult, error) {
	if p.docparseScript == "" {
		return nil, fmt.Errorf("document parsing is unavailable: scripts/run_docparse.py not found")
	}

	args := []string{
		p.docparseScript,
		"--tree",
		"--file_path", filePath,
	}
	result, err := p.runScript(ctx, args)
	if err != nil {
		return nil, err
	}
	return finalizeIndexResult(result, IndexLevelFullTree, "docparse", "", nil), nil
}

// buildDocTreeNative uses the Go-native DOCX engine to build an index tree.
func buildDocTreeNative(filePath string) (*IndexResult, error) {
	pkg, err := docx.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer pkg.Close()

	nodes, err := docx.View(pkg)
	if err != nil {
		return nil, err
	}

	tree := PaperTree{
		DocName:   filepath.Base(filePath),
		Structure: viewNodesToTreeNodes(nodes),
	}

	return &IndexResult{
		Tree:             tree,
		TotalPages:       estimatePages(nodes),
		ModelUsed:        "go-native-docx",
		IndexLevel:       IndexLevelFullTree,
		Extractor:        "go-native-docx",
		TreeAvailable:    true,
		ContentAvailable: false,
	}, nil
}

// viewNodesToTreeNodes converts a flat []ViewNode list into a hierarchical []TreeNode tree
// using heading levels to establish parent-child relationships.
func viewNodesToTreeNodes(nodes []docx.ViewNode) []TreeNode {
	type frame struct {
		node  *TreeNode
		level int
	}

	var roots []TreeNode
	// stack holds open heading frames; stack[0] is the root level
	stack := []frame{}

	nodeIDCounter := 0
	nextID := func() string {
		nodeIDCounter++
		return fmt.Sprintf("node-%d", nodeIDCounter)
	}

	addContent := func(content string) {
		if content == "" {
			return
		}
		if len(stack) == 0 {
			// Content before any heading — attach to a virtual root node
			if len(roots) == 0 {
				roots = append(roots, TreeNode{NodeID: nextID(), Title: "Document"})
			}
			last := &roots[len(roots)-1]
			if last.Summary != "" {
				last.Summary += "\n"
			}
			last.Summary += content
			return
		}
		top := stack[len(stack)-1].node
		if top.Summary != "" {
			top.Summary += "\n"
		}
		top.Summary += content
	}

	for _, n := range nodes {
		switch n.Type {
		case docx.NodeHeading:
			level := n.Level
			if level < 1 {
				level = 1
			}

			newNode := TreeNode{
				NodeID: nextID(),
				Title:  n.Content,
			}

			// Pop stack until we find a frame with lower level
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				popped := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					roots = append(roots, *popped.node)
				} else {
					parent := stack[len(stack)-1].node
					parent.Nodes = append(parent.Nodes, *popped.node)
				}
			}

			stack = append(stack, frame{node: &newNode, level: level})

		case docx.NodeParagraph:
			addContent(n.Content)

		case docx.NodeListItem:
			indent := strings.Repeat("  ", n.Level-1)
			addContent(indent + "• " + n.Content)

		case docx.NodeTable:
			rows := len(n.Children)
			cols := 0
			for _, row := range n.Children {
				if len(row.Children) > cols {
					cols = len(row.Children)
				}
			}
			addContent(fmt.Sprintf("[Table: %dx%d]", rows, cols))
		}
	}

	// Flush remaining stack
	for len(stack) > 0 {
		popped := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			roots = append(roots, *popped.node)
		} else {
			parent := stack[len(stack)-1].node
			parent.Nodes = append(parent.Nodes, *popped.node)
		}
	}

	return roots
}

// estimatePages estimates the number of pages based on total character count.
func estimatePages(nodes []docx.ViewNode) int {
	total := 0
	for _, n := range nodes {
		total += len([]rune(n.Content))
		for _, c := range n.Children {
			total += len([]rune(c.Content))
		}
	}
	pages := total / 2000
	if pages < 1 {
		return 1
	}
	return pages
}

// runScript executes a Python script and parses the JSON output into IndexResult.
func (p *pythonIndexer) runScript(ctx context.Context, args []string) (*IndexResult, error) {
	cmd := exec.CommandContext(ctx, p.pythonPath, args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	// Capture stderr to prevent Python script error output from corrupting the TUI.
	// Without this, stderr inherits the parent process and prints directly to the terminal.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Inject LLM API keys for PageIndex (uses OpenAI SDK internally).
	// PageIndex reads CHATGPT_API_KEY; OpenAI SDK falls back to OPENAI_API_KEY.
	if cfg := config.Get(); cfg != nil {
		for _, prov := range []models.ModelProvider{models.ProviderOpenAI, models.ProviderOpenAICompatible, models.ProviderDeepSeek, models.ProviderSiliconFlow} {
			if p, ok := cfg.Providers[prov]; ok && p.APIKey != "" {
				cmd.Env = append(cmd.Env, "CHATGPT_API_KEY="+p.APIKey, "OPENAI_API_KEY="+p.APIKey)
				if p.BaseURL != "" {
					cmd.Env = append(cmd.Env, "OPENAI_BASE_URL="+p.BaseURL)
				}
				break
			}
		}
	}

	// Confine Python subprocess to a dedicated directory so that
	// PageIndex's hardcoded "./logs" relative path resolves inside
	// .openscholar/logs/pageindex/ instead of polluting the project root.
	if logDir := config.LogDir(); logDir != "" {
		piDir := filepath.Join(logDir, "pageindex")
		if err := os.MkdirAll(piDir, 0o755); err != nil {
			log.Printf("warn: failed to create pageindex dir: %v", err)
		} else {
			cmd.Dir = piDir
		}
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Script exited non-zero. The structured wrapper writes error JSON to stdout
			// even on failure, so try parsing stdout first before falling back to stderr.
			if len(output) > 0 && output[0] == '{' {
				// Likely structured JSON — fall through to parse below
				log.Printf("script exited %d, attempting structured JSON parse", exitErr.ExitCode())
			} else {
				return nil, fmt.Errorf("script failed: %s", stderrBuf.String())
			}
		} else {
			return nil, fmt.Errorf("script execution failed: %w", err)
		}
	}
	// Log stderr warnings if any (non-fatal script output)
	if stderrBuf.Len() > 0 {
		log.Printf("script stderr: %s", stderrBuf.String())
	}

	return parseScriptOutput(output, stderrBuf.String())
}

func parseScriptOutput(output []byte, stderr string) (*IndexResult, error) {
	var raw struct {
		OK           *bool           `json:"ok,omitempty"`
		ErrorType    string          `json:"error_type,omitempty"`
		ErrorMessage string          `json:"error_message,omitempty"`
		Retryable    bool            `json:"retryable,omitempty"`
		Tree         json.RawMessage `json:"tree,omitempty"`
		TotalPages   int             `json:"total_pages,omitempty"`
		TotalTokens  int             `json:"total_tokens,omitempty"`
		ModelUsed    string          `json:"model_used,omitempty"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script output: %w\nstdout: %s\nstderr: %s",
			err, string(output), stderr)
	}

	// Explicit structured error from script wrapper.
	if raw.OK != nil && !*raw.OK {
		if raw.ErrorType != "" {
			return nil, &PageIndexError{
				Type:      raw.ErrorType,
				Message:   raw.ErrorMessage,
				Retryable: raw.Retryable,
			}
		}
		return nil, fmt.Errorf("script returned ok=false with no error_type; stderr: %s", stderr)
	}

	// Legacy success payload may omit "ok", but must include a parseable tree.
	if raw.OK == nil && len(raw.Tree) == 0 {
		return nil, fmt.Errorf("script output missing ok and tree; stderr: %s", stderr)
	}

	var tree PaperTree
	if len(raw.Tree) > 0 {
		if err := json.Unmarshal(raw.Tree, &tree); err != nil {
			return nil, fmt.Errorf("failed to parse tree from script output: %w", err)
		}
	} else if raw.OK != nil && *raw.OK {
		return nil, fmt.Errorf("script returned ok=true without tree; stderr: %s", stderr)
	}

	return &IndexResult{
		Tree:         tree,
		TotalPages:   raw.TotalPages,
		TotalTokens:  raw.TotalTokens,
		ModelUsed:    raw.ModelUsed,
		ContentBytes: contentBytesInTree(tree),
	}, nil
}

func finalizeIndexResult(result *IndexResult, indexLevel, extractor, fallbackReason string, health *PageIndexHealth) *IndexResult {
	if result == nil {
		return nil
	}
	if result.ModelUsed == "" && extractor != "" {
		result.ModelUsed = extractor
	}
	if indexLevel != "" {
		result.IndexLevel = indexLevel
	}
	if extractor != "" {
		result.Extractor = extractor
	}
	if fallbackReason != "" {
		result.FallbackReason = fallbackReason
	}
	if health != nil {
		result.Health = health
	}
	result.ContentBytes = contentBytesInTree(result.Tree)
	result.ContentAvailable = result.ContentBytes > 0
	if result.IndexLevel == "" {
		result.IndexLevel = indexLevelFor(result.ModelUsed, result.ContentAvailable)
	}
	result.TreeAvailable = result.IndexLevel == IndexLevelFullTree
	result.SummaryOnly = result.IndexLevel == IndexLevelSummaryOnly
	result.MissingCapabilities = missingCapabilities(result.IndexLevel, result.ContentAvailable)
	return result
}

func findScript(relPath string) string {
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if _, err := os.Stat(relPath); err == nil {
		abs, _ := filepath.Abs(relPath)
		return abs
	}
	return ""
}
