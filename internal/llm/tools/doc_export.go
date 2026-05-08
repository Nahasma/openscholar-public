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
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type docExportTool struct {
	permissions permission.Service
}

type docExportParams struct {
	InputPath    string `json:"input_path"`
	OutputPath   string `json:"output_path"`
	OutputFormat string `json:"output_format,omitempty"` // docx, md, tex, pdf — auto-detected from output_path if omitted
	ReferenceDoc string `json:"reference_doc,omitempty"`
	Bibliography string `json:"bibliography,omitempty"`
	Profile      string `json:"profile,omitempty"`  // "auto", "latex-project", "markdown-general", "markdown-academic"
	Language     string `json:"language,omitempty"` // "auto", "zh-CN", "en", etc.
}

// NewDocExportTool creates a tool for converting documents between formats.
func NewDocExportTool(perms permission.Service) BaseTool {
	return &docExportTool{permissions: perms}
}

// Available implements AvailabilityChecker. It reports whether DocExport
// can produce useful output based on available system dependencies.
func (t *docExportTool) Available() (bool, string) {
	_, hasPandoc := exec.LookPath("pandoc")

	engines := DetectAllEngines()
	pdfEngineNames := make([]string, 0, len(engines))
	for _, e := range engines {
		pdfEngineNames = append(pdfEngineNames, e.Name)
	}

	font := ResolveCJKFont()

	if hasPandoc != nil && len(engines) == 0 {
		return false, "pandoc not installed and no PDF engine found"
	}
	if hasPandoc != nil {
		return false, "pandoc not installed"
	}

	if len(engines) == 0 {
		return true, "pandoc available but no PDF backend; DOCX/MD/TEX conversion works, PDF export unavailable"
	}

	info := "PDF via " + strings.Join(pdfEngineNames, ", ")
	if font != "" {
		info += "; CJK font: " + font
	} else {
		info += "; no CJK font detected"
	}
	return true, info
}

func (t *docExportTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DocExport",
		Description: "Convert documents between LaTeX (.tex), Word (.docx), Markdown (.md), and PDF formats. Auto-selects the best PDF engine with CJK font support. Supports bibliography handling.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input_path": map[string]any{
					"type":        "string",
					"description": "Path to the input file (.tex, .md, .docx)",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Path for the output file",
				},
				"output_format": map[string]any{
					"type":        "string",
					"description": "Target format: docx, md, tex, pdf. Auto-detected from output_path extension if not specified.",
					"enum":        []string{"docx", "md", "tex", "pdf"},
				},
				"reference_doc": map[string]any{
					"type":        "string",
					"description": "Optional path to a Word reference template (.docx) for styling",
				},
				"bibliography": map[string]any{
					"type":        "string",
					"description": "Optional path to bibliography file (.bib). Auto-detected from input directory if not specified.",
				},
				"profile": map[string]any{
					"type":        "string",
					"description": "Rendering profile: auto (default), latex-project, markdown-general, markdown-academic. Controls PDF engine selection.",
					"enum":        []string{"auto", "latex-project", "markdown-general", "markdown-academic"},
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Document language hint: auto (default), zh-CN, en. Used for font selection.",
				},
			},
			"required": []string{"input_path", "output_path"},
		},
		Required: []string{"input_path", "output_path"},
	}
}

func (t *docExportTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if IsResearchMode(ctx) {
		return NewTextErrorResponse("Research mode is active. Document export is not allowed in research mode. Use Shift+Tab to switch to default mode."), nil
	}

	var params docExportParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.InputPath == "" || params.OutputPath == "" {
		return NewTextErrorResponse("input_path and output_path are required"), nil
	}

	// Permission check for write operation
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "DocExport",
			Description: fmt.Sprintf("Convert %s → %s", params.InputPath, params.OutputPath),
			Action:      "write",
			Path:        params.OutputPath,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	// Determine output format
	outputFormat := params.OutputFormat
	if outputFormat == "" {
		outputFormat = inferFormat(params.OutputPath)
	}

	// PDF output: delegate to the render service for engine fallback + CJK handling
	if outputFormat == "pdf" {
		return t.renderPDF(ctx, params)
	}

	// Non-PDF formats: use pandoc directly
	pandocPath, err := exec.LookPath("pandoc")
	if err != nil {
		if outputFormat == "docx" {
			return t.runPythonExport(ctx, params)
		}
		return NewTextErrorResponse("pandoc not found in PATH. Install with: brew install pandoc (macOS) or apt install pandoc (Linux)"), nil
	}

	args := t.buildPandocArgs(params, outputFormat)
	cmd := exec.CommandContext(ctx, pandocPath, args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Conversion failed: %s\n%s", err, string(output))), nil
	}

	return NewTextResponse(fmt.Sprintf("Document converted successfully:\n- Input: %s\n- Output: %s\n- Format: %s", params.InputPath, params.OutputPath, outputFormat)), nil
}

// renderPDF delegates PDF generation to the render service with engine fallback and CJK support.
func (t *docExportTool) renderPDF(ctx context.Context, params docExportParams) (ToolResponse, error) {
	opts := RenderOptions{
		Profile:  params.Profile,
		Language: params.Language,
	}
	if opts.Profile == "" {
		opts.Profile = "auto"
	}
	if opts.Language == "" {
		opts.Language = "auto"
	}

	result := RenderPDF(ctx, params.InputPath, params.OutputPath, opts)

	if !result.Success {
		var sb strings.Builder
		fmt.Fprintf(&sb, "[%s] %s\n", result.ErrorClass, result.ErrorMessage)
		if len(result.EngineAttempts) > 0 {
			sb.WriteString("\nEngine attempts:\n")
			for _, a := range result.EngineAttempts {
				if a.Error != "" {
					fmt.Fprintf(&sb, "  - %s: FAILED — %s\n", a.Engine, a.Error)
				} else {
					fmt.Fprintf(&sb, "  - %s: OK\n", a.Engine)
				}
			}
		}
		if result.RecommendedAction != "" {
			sb.WriteString("\nRecommended action: " + result.RecommendedAction)
		}
		return NewTextErrorResponse(sb.String()), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "PDF generated successfully:\n- Input: %s\n- Output: %s\n- Engine: %s",
		params.InputPath, result.OutputPath, result.EngineUsed)
	if result.FontUsed != "" {
		fmt.Fprintf(&sb, "\n- Font: %s", result.FontUsed)
	}
	if len(result.Warnings) > 0 {
		sb.WriteString("\n- Warnings:")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "\n  - %s", w)
		}
	}
	return NewTextResponse(sb.String()), nil
}

// buildPandocArgs constructs pandoc command-line arguments for the given conversion.
func (t *docExportTool) buildPandocArgs(params docExportParams, outputFormat string) []string {
	args := []string{
		params.InputPath,
		"-o", params.OutputPath,
	}

	switch outputFormat {
	case "docx":
		if params.ReferenceDoc != "" {
			args = append(args, "--reference-doc", params.ReferenceDoc)
		}
	case "tex":
		args = append(args, "--standalone")
	// PDF is handled by renderPDF(), not buildPandocArgs()
	}

	if params.Bibliography != "" {
		args = append(args, "--citeproc", "--bibliography", params.Bibliography)
	}

	return args
}

// inferFormat determines output format from file extension.
func inferFormat(path string) string {
	switch filepath.Ext(path) {
	case ".docx":
		return "docx"
	case ".md":
		return "md"
	case ".tex":
		return "tex"
	case ".pdf":
		return "pdf"
	default:
		return "docx"
	}
}

// runPythonExport uses the legacy Python script for .docx conversion.
func (t *docExportTool) runPythonExport(ctx context.Context, params docExportParams) (ToolResponse, error) {
	scriptPath := findDocExportScript()
	if scriptPath == "" {
		return NewTextErrorResponse("scripts/run_docexport.py not found and pandoc not available"), nil
	}

	pythonPath := config.PythonPath()
	if pythonPath == "" {
		return NewTextErrorResponse("python3 not found in PATH"), nil
	}

	args := []string{
		scriptPath,
		"--input", params.InputPath,
		"--output", params.OutputPath,
	}
	if params.ReferenceDoc != "" {
		args = append(args, "--reference-doc", params.ReferenceDoc)
	}
	if params.Bibliography != "" {
		args = append(args, "--bib", params.Bibliography)
	}

	cmd := exec.CommandContext(ctx, pythonPath, args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return NewTextErrorResponse(fmt.Sprintf("Export failed: %s", string(exitErr.Stderr))), nil
		}
		return NewTextErrorResponse(fmt.Sprintf("Export execution failed: %v", err)), nil
	}

	var result struct {
		Success    bool     `json:"success"`
		OutputPath string   `json:"output_path"`
		Error      string   `json:"error,omitempty"`
		Warnings   []string `json:"warnings,omitempty"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to parse export result: %v", err)), nil
	}

	if !result.Success {
		return NewTextErrorResponse(fmt.Sprintf("Export failed: %s", result.Error)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Document exported successfully:\n- Output: %s", result.OutputPath)
	if len(result.Warnings) > 0 {
		sb.WriteString("\n- Warnings:")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "\n  - %s", w)
		}
	}
	return NewTextResponse(sb.String()), nil
}

// findDocExportScript locates the run_docexport.py script.
func findDocExportScript() string {
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "scripts", "run_docexport.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	rel := "scripts/run_docexport.py"
	if _, err := os.Stat(rel); err == nil {
		abs, _ := filepath.Abs(rel)
		return abs
	}
	return ""
}
