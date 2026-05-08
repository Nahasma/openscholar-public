package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/docx"
	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true, ".tiff": true, ".tif": true,
	".ico": true, ".svg": false,
}

var documentExtensions = map[string]bool{
	".docx": true,
	".dotx": true,
	".doc":  true,
	".pptx": true,
	".xlsx": true,
}

type viewTool struct{ permissions permission.Service }

type viewParams struct {
	FilePath  string `json:"file_path"`
	Offset    int    `json:"offset,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	StartPage int    `json:"start_page,omitempty"`
	EndPage   int    `json:"end_page,omitempty"`
}

func NewViewTool(perms permission.Service) BaseTool { return &viewTool{permissions: perms} }

func (t *viewTool) Info() ToolInfo {
	return ToolInfo{Name: "View", Description: "Reads a file from the filesystem. Returns the file content with line numbers. Supports offset and limit for large files. For PDF files, supports page-range reading with start_page/end_page.", MaxResultBytes: -1, Parameters: map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string", "description": "Absolute path to the file to read"}, "offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)"}, "limit": map[string]any{"type": "integer", "description": "Maximum number of lines to read"}, "start_page": map[string]any{"type": "integer", "description": "Start page for PDF reading (1-based, inclusive). Only applicable to PDF files."}, "end_page": map[string]any{"type": "integer", "description": "End page for PDF reading (inclusive). Only applicable to PDF files."}}, "required": []string{"file_path"}}, Required: []string{"file_path"}}
}

func (t *viewTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params viewParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	if params.FilePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
	}

	if !IsResearchMode(ctx) {
		if err := ValidateWorkspacePath(ctx, params.FilePath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
		}
	}
	if IsResearchMode(ctx) {
		if err := ValidateResearchPath(ctx, params.FilePath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Research sandbox: %v", err)), nil
		}
		if !filepath.IsAbs(params.FilePath) {
			params.FilePath = filepath.Join(ResearchWorkDir(ctx), params.FilePath)
		}
	}
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{SessionID: sessionID, ToolName: "View", Description: params.FilePath, Action: "read", Path: params.FilePath})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	rp, err := fileop.ResolvePath(params.FilePath, WorkspaceDir(ctx))
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid path: %v", err)), nil
	}
	params.FilePath = rp.Abs
	ext := strings.ToLower(filepath.Ext(params.FilePath))

	if imageExtensions[ext] {
		return NewTextResponse(fmt.Sprintf("This is an image file (%s): %s\nImage files cannot be displayed as text. Use ImageGen to create images or check the file manually.", ext, params.FilePath)), nil
	}
	if ext == ".pdf" {
		return t.readPDF(ctx, params)
	}

	if ext == ".docx" || ext == ".dotx" || ext == ".doc" || documentExtensions[ext] {
		return t.readDocument(ctx, params, ext)
	}

	offset := 0
	if params.Offset > 0 {
		offset = params.Offset - 1
	}
	limit := 2000
	if params.Limit > 0 {
		limit = params.Limit
	}
	readLimits := ReadLimits{DefaultLines: 2000, MaxLineChars: 2000, MaxSizeBytes: 4 * 1024 * 1024}
	if rl, ok := ctx.Value(ReadLimitsContextKey).(ReadLimits); ok {
		readLimits = rl
	}

	if rs, ok := ctx.Value(ReadStateContextKey).(*fileop.ReadState); ok && rs != nil {
		if prev, ok := rs.Get(params.FilePath, offset, limit); ok {
			if st, err := os.Stat(params.FilePath); err == nil && st.ModTime().Equal(prev.MTime) && st.Size() == prev.Size {
				return NewTextResponse("File unchanged since last read; previous content remains in context."), nil
			}
		}
	}

	tr, err := fileop.ReadTextRange(params.FilePath, offset, limit, readLimits.MaxLineChars, readLimits.MaxSizeBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return NewTextErrorResponse(fmt.Sprintf("File not found: %s", params.FilePath)), nil
		}
		return NewTextErrorResponse(fmt.Sprintf("Failed to read file: %v", err)), nil
	}
	if offset > tr.TotalLines {
		return NewTextResponse(fmt.Sprintf("offset %d exceeds file length (%d lines)", offset+1, tr.TotalLines)), nil
	}

	var b strings.Builder
	for i, line := range tr.Lines {
		fmt.Fprintf(&b, "%6d\t%s\n", offset+i+1, line)
	}

	if notifier, ok := ctx.Value(FileReadNotifierContextKey).(FileReadNotifier); ok && notifier != nil {
		sessionID, _ := GetContextValues(ctx)
		if sessionID != "" {
			notifier.OnFileRead(sessionID, params.FilePath, tr.Content)
		}
	}
	if rs, ok := ctx.Value(ReadStateContextKey).(*fileop.ReadState); ok && rs != nil {
		if st, err := os.Stat(params.FilePath); err == nil {
			rs.Put(fileop.FileReadRecord{CanonicalPath: params.FilePath, MTime: st.ModTime(), Size: st.Size(), Offset: offset, Limit: limit, ContentHash: tr.FullHash, Encoding: tr.Encoding, LineEnding: tr.LineEnding, EditEligible: tr.FullHash != ""})
		}
	}
	notifyFileToolUsage(ctx, "View", []string{params.FilePath})
	return NewTextResponse(b.String()), nil
}

func (t *viewTool) readDocument(ctx context.Context, params viewParams, ext string) (ToolResponse, error) {
	var lines []string
	if ext == ".docx" || ext == ".dotx" {
		text, err := viewDocxNative(params.FilePath)
		if err != nil {
			text2, err2 := extractDocumentText(ctx, params.FilePath)
			if err2 != nil {
				return NewTextErrorResponse(fmt.Sprintf("Failed to extract document text: %v (native: %v)", err2, err)), nil
			}
			lines = strings.Split(text2, "\n")
		} else {
			lines = strings.Split(text, "\n")
		}
	} else if ext == ".doc" {
		lines = extractDocLines(ctx, params.FilePath)
		if lines == nil {
			return NewTextErrorResponse("Failed to extract .doc text: no converter or Python parser available"), nil
		}
	} else {
		text, err := extractDocumentText(ctx, params.FilePath)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to extract document text: %v", err)), nil
		}
		lines = strings.Split(text, "\n")
	}
	if rs, ok := ctx.Value(ReadStateContextKey).(*fileop.ReadState); ok && rs != nil {
		if st, err := os.Stat(params.FilePath); err == nil {
			rs.Put(fileop.FileReadRecord{CanonicalPath: params.FilePath, MTime: st.ModTime(), Size: st.Size(), Offset: 0, Limit: 0, ContentHash: "", EditEligible: false})
		}
	}
	notifyFileToolUsage(ctx, "View", []string{params.FilePath})
	return NewTextResponse(renderNumberedLines(lines, params.Offset, params.Limit, 2000)), nil
}

func renderNumberedLines(lines []string, offsetParam, limitParam, maxLineChars int) string {
	offset := 0
	if offsetParam > 0 {
		offset = offsetParam - 1
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	limit := 2000
	if limitParam > 0 {
		limit = limitParam
	}
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	var result strings.Builder
	for i := offset; i < end; i++ {
		line := lines[i]
		if maxLineChars > 0 && len(line) > maxLineChars {
			line = line[:maxLineChars] + "..."
		}
		fmt.Fprintf(&result, "%6d\t%s\n", i+1, line)
	}
	return result.String()
}

func (t *viewTool) readPDF(ctx context.Context, params viewParams) (ToolResponse, error) {
	totalPages, err := getPDFPageCount(params.FilePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to read PDF: %v", err)), nil
	}
	startPage := 1
	endPage := totalPages
	if params.StartPage > 0 {
		startPage = params.StartPage
	}
	if params.EndPage > 0 {
		endPage = params.EndPage
	}
	if startPage > totalPages {
		startPage = totalPages
	}
	if endPage > totalPages {
		endPage = totalPages
	}
	if startPage > endPage {
		startPage = endPage
	}
	pageCount := endPage - startPage + 1
	estimatedTokens := pageCount * 650
	text, err := extractPDFPages(ctx, params.FilePath, startPage, endPage)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to extract PDF text: %v", err)), nil
	}
	var header strings.Builder
	fmt.Fprintf(&header, "PDF: %d/%d pages (p%d-p%d), ~%d tokens\n", pageCount, totalPages, startPage, endPage, estimatedTokens)
	if totalPages > 30 && params.StartPage == 0 && params.EndPage == 0 {
		header.WriteString("Tip: This PDF has many pages. Consider using start_page/end_page to read specific sections.\n")
	}
	header.WriteString("---\n")
	if rs, ok := ctx.Value(ReadStateContextKey).(*fileop.ReadState); ok && rs != nil {
		if st, err := os.Stat(params.FilePath); err == nil {
			rs.Put(fileop.FileReadRecord{CanonicalPath: params.FilePath, MTime: st.ModTime(), Size: st.Size(), Offset: 0, Limit: 0, ContentHash: "", EditEligible: false})
		}
	}
	notifyFileToolUsage(ctx, "View", []string{params.FilePath})
	return NewTextResponse(header.String() + text), nil
}

// getPDFPageCount returns the total number of pages in a PDF file using PyMuPDF.
func getPDFPageCount(filePath string) (int, error) {
	pythonPath := config.PythonPath()
	if pythonPath == "" {
		return 0, fmt.Errorf("python3 not found")
	}

	script := `import fitz,sys,json;doc=fitz.open(sys.argv[1]);print(json.dumps({"pages":len(doc)}))`
	cmd := exec.Command(pythonPath, "-c", script, filePath)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return 0, fmt.Errorf("PDF page count failed: %s", string(exitErr.Stderr))
		}
		return 0, fmt.Errorf("PDF page count failed: %w", err)
	}

	var result struct {
		Pages int `json:"pages"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("failed to parse page count: %w", err)
	}
	return result.Pages, nil
}

// extractPDFPages extracts text from a page range of a PDF file using PyMuPDF.
func extractPDFPages(ctx context.Context, filePath string, startPage, endPage int) (string, error) {
	pythonPath := config.PythonPath()
	if pythonPath == "" {
		return "", fmt.Errorf("python3 not found")
	}

	script := `
import fitz, sys
doc = fitz.open(sys.argv[1])
start, end = int(sys.argv[2]), int(sys.argv[3])
for i in range(start - 1, end):
    if i < len(doc):
        print(f"--- Page {i+1} ---")
        print(doc[i].get_text())
`
	cmd := exec.CommandContext(ctx, pythonPath, "-c", script, filePath,
		fmt.Sprintf("%d", startPage), fmt.Sprintf("%d", endPage))
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("PDF extraction failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("PDF extraction failed: %w", err)
	}
	return string(output), nil
}

// viewDocxNative opens a .docx/.dotx file with the Go-native engine
// and renders it as terminal-friendly text.
func viewDocxNative(filePath string) (string, error) {
	pkg, err := docx.Open(filePath)
	if err != nil {
		return "", err
	}
	defer pkg.Close()

	nodes, err := docx.View(pkg)
	if err != nil {
		return "", err
	}

	return docx.RenderTerminal(nodes, 120), nil
}

// extractDocLines handles .doc files: normalize to .docx and use native viewer,
// falling back to Python. Returns nil if all methods fail.
func extractDocLines(ctx context.Context, filePath string) []string {
	if docx.IsNormalizationAvailable() {
		if docxPath, err := docx.NormalizeOfficeInput(ctx, filePath); err == nil {
			if text, err := viewDocxNative(docxPath); err == nil {
				return strings.Split(text, "\n")
			}
		}
	}
	text, err := extractDocumentText(ctx, filePath)
	if err != nil {
		return nil
	}
	return strings.Split(text, "\n")
}

// extractDocumentText calls the Python docparse script to extract plain text
// from binary document formats (DOCX, PPTX, XLSX).
func extractDocumentText(ctx context.Context, filePath string) (string, error) {
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return "", fmt.Errorf("python3 not found in PATH: %w", err)
	}

	scriptPath := findDocparseScript()
	if scriptPath == "" {
		return "", fmt.Errorf("scripts/run_docparse.py not found. Document text extraction is unavailable")
	}

	cmd := exec.CommandContext(ctx, pythonPath, scriptPath, "--extract", "--file_path", filePath)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("docparse failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("docparse execution failed: %w", err)
	}
	return string(output), nil
}

// findDocparseScript locates the run_docparse.py script.
func findDocparseScript() string {
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "scripts", "run_docparse.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	rel := "scripts/run_docparse.py"
	if _, err := os.Stat(rel); err == nil {
		abs, _ := filepath.Abs(rel)
		return abs
	}
	return ""
}
