package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/docx"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type docxEditTool struct {
	permissions permission.Service
}

type docxEditParams struct {
	FilePath   string              `json:"file_path"`
	Operations []docxEditOperation `json:"operations"`
	OutputPath string              `json:"output_path,omitempty"`
}

type docxEditOperation struct {
	Type    string            `json:"type"`    // "replace_text", "insert_after", "insert_before", "delete", "replace_section"
	Target  docxEditTarget    `json:"target"`
	Content string            `json:"content,omitempty"`
	Style   map[string]string `json:"style,omitempty"`
}

type docxEditTarget struct {
	ParaID   string `json:"para_id,omitempty"`
	Bookmark string `json:"bookmark,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// NewDocxEditTool creates a tool for applying local edits to DOCX documents.
func NewDocxEditTool(perms permission.Service) BaseTool {
	return &docxEditTool{permissions: perms}
}

func (t *docxEditTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DocxEdit",
		Description: "Edit a DOCX document by applying operations (replace text, insert paragraphs, delete paragraphs, replace sections). Each operation targets a specific location using paragraph ID, bookmark name, or offset.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the DOCX file to edit.",
				},
				"operations": map[string]any{
					"type":        "array",
					"description": "Ordered list of edit operations to apply.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type": map[string]any{
								"type":        "string",
								"enum":        []string{"replace_text", "insert_after", "insert_before", "delete", "replace_section"},
								"description": "The kind of edit: replace_text replaces paragraph text; insert_after/insert_before inserts a new paragraph; delete removes the paragraph; replace_section replaces all body paragraphs under a heading.",
							},
							"target": map[string]any{
								"type":        "object",
								"description": "Locates the target paragraph. Specify one of: para_id (w14:paraId attribute), bookmark (bookmark name), or offset (0-based paragraph index).",
								"properties": map[string]any{
									"para_id":  map[string]any{"type": "string", "description": "The w14:paraId attribute value of the target paragraph."},
									"bookmark": map[string]any{"type": "string", "description": "Bookmark name (w:bookmarkStart w:name) inside the target paragraph."},
									"offset":   map[string]any{"type": "integer", "description": "0-based index of the target paragraph among all paragraphs in the body."},
								},
							},
							"content": map[string]any{
								"type":        "string",
								"description": "New text content for replace_text, insert_after, insert_before, and replace_section operations.",
							},
							"style": map[string]any{
								"type":        "object",
								"description": `Optional style overrides for the inserted/replaced run. Supported keys: "bold" ("true"/"false"), "italic", "underline", "strike", "font" (font name), "size" (half-points as string).`,
							},
						},
						"required": []string{"type", "target"},
					},
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Path to write the edited file. Defaults to overwriting file_path in place.",
				},
			},
			"required": []string{"file_path", "operations"},
		},
		Required: []string{"file_path", "operations"},
	}
}

func (t *docxEditTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p docxEditParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if p.FilePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
	}
	if len(p.Operations) == 0 {
		return NewTextErrorResponse("operations list is empty; provide at least one operation"), nil
	}

	// Path sandbox validation
	if err := ValidateWorkspacePath(ctx, p.FilePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Path validation failed: %v", err)), nil
	}
	if p.OutputPath != "" {
		if err := ValidateWorkspacePath(ctx, p.OutputPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Path validation failed: %v", err)), nil
		}
	}

	outputPath := p.OutputPath
	if outputPath == "" {
		outputPath = p.FilePath
	}

	// Permission check for write operation
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "DocxEdit",
			Description: fmt.Sprintf("Edit DOCX %s → %s (%d operation(s))", p.FilePath, outputPath, len(p.Operations)),
			Action:      "write",
			Path:        outputPath,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	pkg, err := docx.Open(p.FilePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to open %q: %v", p.FilePath, err)), nil
	}
	defer pkg.Close()

	// Convert API operation structs → docx.EditOperation
	ops, convErr := convertOperations(p.Operations)
	if convErr != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid operations: %v", convErr)), nil
	}

	result, err := docx.Apply(pkg, ops)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Apply failed: %v", err)), nil
	}

	if err := pkg.Save(outputPath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to save %q: %v", outputPath, err)), nil
	}

	return NewTextResponse(formatEditResult(result, p.FilePath, outputPath)), nil
}

// convertOperations converts []docxEditOperation to []docx.EditOperation.
func convertOperations(ops []docxEditOperation) ([]docx.EditOperation, error) {
	out := make([]docx.EditOperation, 0, len(ops))
	for i, op := range ops {
		opType, err := parseOpType(op.Type)
		if err != nil {
			return nil, fmt.Errorf("op[%d]: %w", i, err)
		}
		out = append(out, docx.EditOperation{
			Type: opType,
			Target: docx.StableAnchor{
				ParaID:   op.Target.ParaID,
				Bookmark: op.Target.Bookmark,
				Offset:   op.Target.Offset,
			},
			Content: op.Content,
			Style:   op.Style,
		})
	}
	return out, nil
}

// parseOpType maps a string operation type to the docx.EditOpType constant.
func parseOpType(s string) (docx.EditOpType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "replace_text":
		return docx.OpReplaceText, nil
	case "insert_after":
		return docx.OpInsertAfter, nil
	case "insert_before":
		return docx.OpInsertBefore, nil
	case "delete":
		return docx.OpDeleteParagraph, nil
	case "replace_section":
		return docx.OpReplaceSection, nil
	default:
		return 0, fmt.Errorf("unknown operation type %q; allowed: replace_text, insert_after, insert_before, delete, replace_section", s)
	}
}

// formatEditResult formats the EditResult into a human-readable summary.
func formatEditResult(result *docx.EditResult, filePath, outputPath string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "DocxEdit completed\n")
	fmt.Fprintf(&sb, "  Input:   %s\n", filePath)
	fmt.Fprintf(&sb, "  Output:  %s\n", outputPath)
	fmt.Fprintf(&sb, "  Applied: %d operation(s)\n", result.Applied)
	fmt.Fprintf(&sb, "  Skipped: %d operation(s)\n", result.Skipped)

	if len(result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "  - %s\n", w)
		}
	}

	return sb.String()
}
