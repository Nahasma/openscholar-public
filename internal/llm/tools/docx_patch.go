package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/docx"
	"github.com/openscholar/openscholar/internal/docx/template"
	"github.com/openscholar/openscholar/internal/permission"
)

type docxPatchTool struct {
	permissions permission.Service
}

type docxPatchParams struct {
	TemplatePath string         `json:"template_path"`
	OutputPath   string         `json:"output_path,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
	Analyze      bool           `json:"analyze,omitempty"`
}

// NewDocxPatchTool creates a tool for analyzing and filling DOCX templates.
func NewDocxPatchTool(perms permission.Service) BaseTool {
	return &docxPatchTool{permissions: perms}
}

func (t *docxPatchTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DocxPatch",
		Description: "Analyze DOCX templates and fill them with data. Use analyze=true to inspect a template's slots (placeholders, content controls, bookmarks) before filling. Use with data to fill the template and save the result.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"template_path": map[string]any{
					"type":        "string",
					"description": "Path to the DOCX template file (.docx or .dotx)",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Path for the filled output file. If omitted in fill mode, the template file is overwritten in-place.",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Key-value map of slot names to fill. Values can be strings (text slots), booleans (conditional slots), arrays of objects (loop slots), or image paths (image slots).",
				},
				"analyze": map[string]any{
					"type":        "boolean",
					"description": "When true, inspect the template and return its slot structure without modifying any file. Default: false.",
				},
			},
			"required": []string{"template_path"},
		},
		Required: []string{"template_path"},
	}
}

func (t *docxPatchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p docxPatchParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if p.TemplatePath == "" {
		return NewTextErrorResponse("template_path is required"), nil
	}

	// Path sandbox validation
	if err := ValidateWorkspacePath(ctx, p.TemplatePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Path validation failed: %v", err)), nil
	}
	if p.OutputPath != "" {
		if err := ValidateWorkspacePath(ctx, p.OutputPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Path validation failed: %v", err)), nil
		}
	}

	// ── Analyze mode ─────────────────────────────────────────────────────────
	if p.Analyze {
		pkg, err := docx.Open(p.TemplatePath)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to open template %q: %v", p.TemplatePath, err)), nil
		}
		defer pkg.Close()

		spec, err := template.Analyze(pkg)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to analyze template: %v", err)), nil
		}

		return NewTextResponse(formatTemplateSpec(spec)), nil
	}

	// ── Fill mode ─────────────────────────────────────────────────────────────
	outputPath := p.OutputPath
	if outputPath == "" {
		outputPath = p.TemplatePath
	}

	// Permission check for write operation
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "DocxPatch",
			Description: fmt.Sprintf("Fill DOCX template %s → %s", p.TemplatePath, outputPath),
			Action:      "write",
			Path:        outputPath,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	pkg, err := docx.Open(p.TemplatePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to open template %q: %v", p.TemplatePath, err)), nil
	}
	defer pkg.Close()

	spec, err := template.Analyze(pkg)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to analyze template: %v", err)), nil
	}

	data := p.Data
	if data == nil {
		data = make(map[string]any)
	}

	result, err := template.Fill(pkg, spec, data)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to fill template: %v", err)), nil
	}

	if err := pkg.Save(outputPath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to save output to %q: %v", outputPath, err)), nil
	}

	return NewTextResponse(formatFillResult(result, p.TemplatePath, outputPath)), nil
}

// formatTemplateSpec renders a TemplateSpec as human-readable text for the LLM.
func formatTemplateSpec(spec *template.TemplateSpec) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Template Analysis\n")
	fmt.Fprintf(&sb, "=================\n")

	// Metadata summary
	if len(spec.Metadata) > 0 {
		sb.WriteString("\nMetadata:\n")
		for k, v := range spec.Metadata {
			fmt.Fprintf(&sb, "  %s: %s\n", k, v)
		}
	}

	// Slots
	if len(spec.Slots) == 0 {
		sb.WriteString("\nSlots: none\n")
	} else {
		fmt.Fprintf(&sb, "\nSlots (%d):\n", len(spec.Slots))
		for _, slot := range spec.Slots {
			required := ""
			if slot.Required {
				required = " [required]"
			}
			defaultVal := ""
			if slot.Default != "" {
				defaultVal = fmt.Sprintf(" (default: %q)", slot.Default)
			}
			fmt.Fprintf(&sb, "  - %s  type=%s  anchor=%s%s%s\n",
				slot.Name,
				slotDataTypeName(slot.DataType),
				anchorTypeName(slot.AnchorType),
				required,
				defaultVal,
			)
		}
	}

	// Protected zones
	if len(spec.ProtectedZones) > 0 {
		fmt.Fprintf(&sb, "\nProtected Zones (%d):\n", len(spec.ProtectedZones))
		for _, z := range spec.ProtectedZones {
			fmt.Fprintf(&sb, "  - %s\n", z.Name)
		}
	}

	// Section structure
	if len(spec.Sections) > 0 {
		sb.WriteString("\nSection Structure:\n")
		writeSections(&sb, spec.Sections, 0)
	}

	return sb.String()
}

// writeSections recursively writes the section tree with indentation.
func writeSections(sb *strings.Builder, sections []template.SectionSpec, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, s := range sections {
		fmt.Fprintf(sb, "%s  [H%d] %s\n", indent, s.Level, s.Title)
		if len(s.Children) > 0 {
			writeSections(sb, s.Children, depth+1)
		}
	}
}

// formatFillResult renders a FillResult summary as human-readable text.
func formatFillResult(result *template.FillResult, templatePath, outputPath string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Template filled successfully\n")
	fmt.Fprintf(&sb, "  Template: %s\n", templatePath)
	fmt.Fprintf(&sb, "  Output:   %s\n", outputPath)
	fmt.Fprintf(&sb, "  Replaced: %d slot(s)\n", len(result.Replaced))

	if len(result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "  - %s\n", w)
		}
	}

	if len(result.Errors) > 0 {
		sb.WriteString("\nErrors (missing required slots):\n")
		for _, e := range result.Errors {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
	}

	if len(result.Replaced) > 0 {
		sb.WriteString("\nReplaced slots:\n")
		for _, r := range result.Replaced {
			status := "OK"
			if !r.Success {
				status = "FAILED"
			}
			fmt.Fprintf(&sb, "  [%s] %s → %q\n", status, r.Placeholder, r.NewValue)
		}
	}

	return sb.String()
}

// anchorTypeName returns a short label for an AnchorType.
func anchorTypeName(a template.AnchorType) string {
	switch a {
	case template.AnchorContentControl:
		return "content-control"
	case template.AnchorBookmark:
		return "bookmark"
	case template.AnchorPlaceholder:
		return "placeholder"
	default:
		return "unknown"
	}
}

// slotDataTypeName returns a short label for a SlotDataType.
// (mirrors the unexported helper in the template package for formatting)
func slotDataTypeName(dt template.SlotDataType) string {
	switch dt {
	case template.SlotText:
		return "text"
	case template.SlotRichText:
		return "richtext"
	case template.SlotImage:
		return "image"
	case template.SlotTable:
		return "table"
	case template.SlotConditional:
		return "conditional"
	case template.SlotLoop:
		return "loop"
	default:
		return "unknown"
	}
}
