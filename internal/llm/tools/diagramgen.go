package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/permission"
)

type diagramGenTool struct {
	permissions permission.Service
}

type diagramGenParams struct {
	Code          string `json:"code"`
	Filename      string `json:"filename"`
	Directory     string `json:"directory,omitempty"`
	Theme         int    `json:"theme,omitempty"`
	Format        string `json:"format,omitempty"`
	StylePreset   string `json:"style_preset,omitempty"`
	StrictQuality *bool  `json:"strict_quality,omitempty"`
}

func NewDiagramGenTool(perms permission.Service) BaseTool {
	return &diagramGenTool{permissions: perms}
}

func (t *diagramGenTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DiagramGen",
		Description: "Generate technical diagrams from D2 code. Use for architecture diagrams, flowcharts, sequence diagrams, class diagrams, and other figures requiring precise text labels. D2 renders text accurately via vector engine — no garbled text. Output is SVG+PNG.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{
					"type":        "string",
					"description": "D2 diagram source code. See https://d2lang.com/ for syntax reference. Key syntax: 'A -> B' for connections, 'A: Label' for nodes, '{}' for containers, 'direction: right/down' for layout.",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Output filename without extension (e.g. 'agent_architecture'). Saved as <filename>.svg and <filename>.png in the output directory.",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Optional output directory path. If omitted, saves to the current working directory.",
				},
				"theme": map[string]any{
					"type":        "integer",
					"description": "D2 theme ID. 0=auto, 1=Neutral Grey, 3=Flagship Terrastruct, 4=Cool Classics, 100=dark. Default: auto paper theme (3).",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Output format preference: both|svg|png. SVG is always rendered from D2 source; PNG is generated when requested.",
				},
				"style_preset": map[string]any{
					"type":        "string",
					"description": "Style preset: paper|minimal|none. Default: paper.",
				},
				"strict_quality": map[string]any{
					"type":        "boolean",
					"description": "When true, ambiguous nested-node references fail with actionable errors. Default: true.",
				},
			},
			"required": []string{"code", "filename"},
		},
		Required: []string{"code", "filename"},
	}
}

func (t *diagramGenTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if IsResearchMode(ctx) {
		return NewTextErrorResponse("Research mode is active. Diagram generation is not allowed in research mode. Use Shift+Tab to switch to default mode."), nil
	}

	var params diagramGenParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.Code == "" {
		return NewTextErrorResponse("code is required"), nil
	}
	if params.Filename == "" {
		return NewTextErrorResponse("filename is required"), nil
	}
	params.Filename = strings.TrimSpace(params.Filename)
	if err := validateArtifactFilename(params.Filename); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid filename: %v", err)), nil
	}

	format := strings.ToLower(strings.TrimSpace(params.Format))
	if format == "" {
		format = "both"
	}
	if format != "both" && format != "svg" && format != "png" {
		return NewTextErrorResponse("format must be one of: both, svg, png"), nil
	}

	stylePreset := strings.ToLower(strings.TrimSpace(params.StylePreset))
	if stylePreset == "" {
		stylePreset = "paper"
	}
	if stylePreset != "paper" && stylePreset != "minimal" && stylePreset != "none" {
		return NewTextErrorResponse("style_preset must be one of: paper, minimal, none"), nil
	}
	strictQuality := true
	if params.StrictQuality != nil {
		strictQuality = *params.StrictQuality
	}

	// Permission check
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		codePreview := params.Code
		if len(codePreview) > 80 {
			codePreview = codePreview[:80] + "..."
		}
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "DiagramGen",
			Description: fmt.Sprintf("Generate diagram: %s → %s.svg", codePreview, params.Filename),
			Action:      "write",
			Params:      params,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	// Check if d2 is installed
	d2Path, err := exec.LookPath("d2")
	if err != nil {
		return NewTextErrorResponse(
			"D2 CLI not found. Install it:\n" +
				"  macOS: brew install d2\n" +
				"  Linux: curl -fsSL https://d2lang.com/install.sh | sh\n" +
				"  Or see: https://github.com/terrastruct/d2#install",
		), nil
	}

	// Determine output directory
	cwd, _ := os.Getwd()
	outDir := cwd
	if params.Directory != "" {
		if filepath.IsAbs(params.Directory) {
			outDir = params.Directory
		} else {
			outDir = filepath.Join(cwd, params.Directory)
		}
	}
	if err := ValidateWorkspacePath(ctx, outDir); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to create output directory: %v", err)), nil
	}

	normalizedCode, qualityWarnings, qualityErrors := normalizeAndLintD2(params.Code, strictQuality)
	if len(qualityErrors) > 0 {
		return NewTextErrorResponse("D2 quality check failed:\n- " + strings.Join(qualityErrors, "\n- ")), nil
	}
	if stylePreset == "paper" {
		normalizedCode = applyPaperD2Style(normalizedCode)
	}

	d2PathOut := filepath.Join(outDir, params.Filename+".d2")
	svgPath := filepath.Join(outDir, params.Filename+".svg")
	pngPath := filepath.Join(outDir, params.Filename+".png")
	for _, outPath := range []string{d2PathOut, svgPath, pngPath} {
		if err := ValidateWorkspacePath(ctx, outPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
		}
	}
	if err := os.WriteFile(d2PathOut, []byte(normalizedCode), 0644); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to save D2 source: %v", err)), nil
	}

	theme := params.Theme
	if theme == 0 {
		theme = defaultThemeForDiagramStyle(stylePreset)
	}

	// Render SVG
	args := []string{d2PathOut, svgPath}
	if theme != 0 {
		args = append([]string{"--theme", fmt.Sprintf("%d", theme)}, args...)
	}

	cmd := exec.CommandContext(ctx, d2Path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("D2 rendering failed:\n%s\n%v", string(output), err)), nil
	}

	// Prefer direct D2 PNG output, then SVG conversion fallbacks.
	pngGenerated := false
	if format == "both" || format == "png" {
		pngArgs := []string{d2PathOut, pngPath}
		if theme != 0 {
			pngArgs = append([]string{"--theme", fmt.Sprintf("%d", theme)}, pngArgs...)
		}
		cmd := exec.CommandContext(ctx, d2Path, pngArgs...)
		if err := cmd.Run(); err == nil {
			pngGenerated = true
		}
	}

	if (format == "both" || format == "png") && !pngGenerated {
		if rsvgPath, err := exec.LookPath("rsvg-convert"); err == nil {
			cmd := exec.CommandContext(ctx, rsvgPath, "-o", pngPath, "--dpi", "300", svgPath)
			if err := cmd.Run(); err == nil {
				pngGenerated = true
			}
		}

		if !pngGenerated {
			if inkscapePath, err := exec.LookPath("inkscape"); err == nil {
				cmd := exec.CommandContext(ctx, inkscapePath, svgPath, "--export-type=png", "--export-filename="+pngPath, "--export-dpi=300")
				if err := cmd.Run(); err == nil {
					pngGenerated = true
				}
			}
		}
	}

	// Build result
	var result strings.Builder
	fmt.Fprintf(&result, "D2 source saved to: %s\n", d2PathOut)
	fmt.Fprintf(&result, "Diagram saved to: %s\n", svgPath)
	if format == "both" || format == "png" {
		if pngGenerated {
			fmt.Fprintf(&result, "PNG exported to: %s\n", pngPath)
		} else {
			fmt.Fprintf(&result, "PNG export unavailable in current environment (SVG is ready).\n")
		}
	}
	if len(qualityWarnings) > 0 {
		fmt.Fprintf(&result, "\nQuality notes:\n- %s\n", strings.Join(qualityWarnings, "\n- "))
	}

	// LaTeX usage hint — prefer PNG if available, otherwise SVG
	imgFile := params.Filename + ".svg"
	if pngGenerated && (format == "both" || format == "png") {
		imgFile = params.Filename + ".png"
	}
	relDir, _ := filepath.Rel(cwd, outDir)
	if relDir == "" || relDir == "." {
		relDir = ""
	}
	var relPath string
	if relDir != "" {
		relPath = filepath.Join(relDir, imgFile)
	} else {
		relPath = imgFile
	}
	fmt.Fprintf(&result, "\nLaTeX usage:\n\\begin{figure}[htbp]\n\\centering\n\\includegraphics[width=0.8\\textwidth]{%s}\n\\caption{TODO: Add caption}\n\\label{fig:%s}\n\\end{figure}\n", relPath, params.Filename)
	result.WriteString("\nCompletion nudge: Reply with the saved paths above and continue writing. Do not run extra d2/file existence checks.\n")

	artifactPaths := []string{d2PathOut, svgPath}
	actualFormat := "svg"
	if pngGenerated && (format == "both" || format == "png") {
		artifactPaths = append(artifactPaths, pngPath)
		actualFormat = "svg+png"
	}
	metadata := map[string]any{
		"durable_progress": true,
		"artifact_kind":    "diagram",
		"d2_path":          d2PathOut,
		"svg_path":         svgPath,
		"artifact_paths":   artifactPaths,
		"format":           actualFormat,
		"quality_warnings": qualityWarnings,
	}
	if pngGenerated {
		metadata["png_path"] = pngPath
	}

	return WithResponseMetadata(NewTextResponse(result.String()), metadata), nil
}

func defaultThemeForDiagramStyle(stylePreset string) int {
	if stylePreset == "none" {
		return 0
	}
	return 3
}

func applyPaperD2Style(code string) string {
	if strings.Contains(code, "style:") {
		return code
	}
	prefix := "vars: {\n  d2-config: {\n    sketch: false\n  }\n}\n\n"
	return prefix + code
}
