package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/docx"
	"github.com/Nahasma/openscholar-public/internal/docx/validate"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type docxValidateTool struct {
	permissions permission.Service
}

type docxValidateParams struct {
	FilePath string `json:"file_path"`
	DocType  string `json:"doc_type,omitempty"`
	AutoFix  bool   `json:"auto_fix,omitempty"`
}

// NewDocxValidateTool creates a tool for validating DOCX documents.
func NewDocxValidateTool(perms permission.Service) BaseTool {
	return &docxValidateTool{permissions: perms}
}

func (t *docxValidateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DocxValidate",
		Description: "Validate a DOCX document against format and content rules. Optionally specify doc_type (patent/paper/proposal) for domain-specific rules. Returns diagnostics with severity levels, locations, and fix suggestions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the DOCX file to validate.",
				},
				"doc_type": map[string]any{
					"type":        "string",
					"description": "Optional document type for domain-specific rules: 'patent', 'paper', or 'proposal'. If omitted, only generic format and content rules are applied.",
				},
				"auto_fix": map[string]any{
					"type":        "boolean",
					"description": "When true, report which diagnostics support automatic fixing. Default: false.",
				},
			},
			"required": []string{"file_path"},
		},
		Required: []string{"file_path"},
	}
}

func (t *docxValidateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p docxValidateParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if p.FilePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
	}

	// Path sandbox validation
	if err := ValidateWorkspacePath(ctx, p.FilePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Path validation failed: %v", err)), nil
	}

	pkg, err := docx.Open(p.FilePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to open document %q: %v", p.FilePath, err)), nil
	}
	defer pkg.Close()

	nodes, err := docx.View(pkg)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to read document structure: %v", err)), nil
	}

	// Select rule set based on whether a doc type was provided.
	var rules []validate.Rule
	if p.DocType != "" {
		rules = validate.AllRules()
	} else {
		rules = validate.DefaultRules()
	}

	diags := validate.NewValidator(rules...).Validate(pkg, nodes, p.DocType)

	return NewTextResponse(formatValidateResult(diags, p.FilePath, p.DocType, p.AutoFix)), nil
}

// formatValidateResult renders diagnostics as human-readable text for the LLM.
func formatValidateResult(diags []validate.Diagnostic, filePath, docType string, showAutoFix bool) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "DocxValidate Results: %s\n", filePath)
	if docType != "" {
		fmt.Fprintf(&sb, "Doc Type: %s\n", docType)
	}
	sb.WriteString("\n")

	if len(diags) == 0 {
		sb.WriteString("No issues found. Document passed all validation rules.\n")
		return sb.String()
	}

	// Count by level.
	var errCount, warnCount, infoCount int
	for _, d := range diags {
		switch d.Level {
		case validate.DiagError:
			errCount++
		case validate.DiagWarning:
			warnCount++
		case validate.DiagInfo:
			infoCount++
		}
	}

	fmt.Fprintf(&sb, "Total: %d (%d error, %d warning, %d info)\n", len(diags), errCount, warnCount, infoCount)

	for _, d := range diags {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, "[%s] %s: %s\n", strings.ToUpper(d.Level.String()), d.RuleID, d.Message)

		// Location
		loc := formatAnchor(d.Location)
		if loc != "" {
			fmt.Fprintf(&sb, "  Location: %s\n", loc)
		}

		if d.Suggestion != "" {
			fmt.Fprintf(&sb, "  Suggestion: %s\n", d.Suggestion)
		}

		if showAutoFix && d.AutoFix {
			sb.WriteString("  AutoFix: available\n")
		}
	}

	return sb.String()
}

// formatAnchor converts a StableAnchor to a concise location string.
func formatAnchor(a docx.StableAnchor) string {
	if a.Part == "" {
		return ""
	}
	loc := a.Part
	if a.ParaID != "" {
		loc += fmt.Sprintf(" (paraId: %s)", a.ParaID)
	}
	return loc
}
