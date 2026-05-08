package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
	"github.com/openscholar/openscholar/internal/permission"
)

type editTool struct {
	permissions permission.Service
}

type editParams struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func NewEditTool(perms permission.Service) BaseTool {
	return &editTool{permissions: perms}
}

func (t *editTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "Edit",
		Description: "Performs exact string replacement in a file. The old_string must be unique in the file unless replace_all is true.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to modify",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "The text to replace",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "The replacement text",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences (default false)",
				},
			},
			"required": []string{"file_path", "old_string", "new_string"},
		},
		Required: []string{"file_path", "old_string", "new_string"},
	}
}

func (t *editTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params editParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	// S3: Reject glob patterns in write target path
	if err := validateWriteTarget(params.FilePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid file_path: %v", err)), nil
	}

	// Universal workspace boundary (normal mode)
	if !IsResearchMode(ctx) {
		if err := ValidateWorkspacePath(ctx, params.FilePath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
		}
	}

	// Research mode: sandbox to workspace directory (replaces blanket block)
	if IsResearchMode(ctx) {
		if err := ValidateResearchPath(ctx, params.FilePath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Research sandbox: %v", err)), nil
		}
		if !filepath.IsAbs(params.FilePath) {
			params.FilePath = filepath.Join(ResearchWorkDir(ctx), params.FilePath)
		}
	}
	rp, err := fileop.ResolvePath(params.FilePath, WorkspaceDir(ctx))
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid file_path: %v", err)), nil
	}
	params.FilePath = rp.Abs

	// Permission check: Edit is a write operation
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "Edit",
			Description: params.FilePath,
			Action:      "edit",
			Path:        params.FilePath,
			Params:      params,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	data, err := os.ReadFile(params.FilePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to read file: %v", err)), nil
	}
	if err := enforceStaleGuard(ctx, params.FilePath, data); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	content := string(data)

	if params.OldString == params.NewString {
		return NewTextErrorResponse("old_string and new_string must be different"), nil
	}

	count := strings.Count(content, params.OldString)
	if count == 0 {
		return NewTextErrorResponse("old_string not found in file"), nil
	}
	if count > 1 && !params.ReplaceAll {
		return NewTextErrorResponse(fmt.Sprintf("old_string found %d times. Use replace_all or provide more context to make it unique.", count)), nil
	}

	var newContent string
	if params.ReplaceAll {
		newContent = strings.ReplaceAll(content, params.OldString, params.NewString)
	} else {
		newContent = strings.Replace(content, params.OldString, params.NewString, 1)
	}
	if strings.Contains(content, "\r\n") && !strings.Contains(newContent, "\r\n") {
		newContent = strings.ReplaceAll(newContent, "\n", "\r\n")
	}
	if strings.HasPrefix(content, "\uFEFF") && !strings.HasPrefix(newContent, "\uFEFF") {
		newContent = "\uFEFF" + newContent
	}

	// Phase 5: capture file snapshot before writing
	captureCheckpointBeforeWrite(ctx, params.FilePath)

	if err := fileop.WriteFileAtomic(params.FilePath, []byte(newContent), 0o644); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to write file: %v", err)), nil
	}
	notifyFileToolUsage(ctx, "Edit", []string{params.FilePath})

	// Lint guard: run post-write checks if enabled
	if cfg := config.Get(); cfg != nil && cfg.Harness.LintGuardEnabled {
		guard := NewLintGuard(cfg.Harness.LintGuardLatex, cfg.Harness.LintGuardGo)
		if result := guard.Check(params.FilePath); result != nil && !result.Passed {
			// Roll back to original content
			_ = fileop.WriteFileAtomic(params.FilePath, data, 0o644)
			refreshReadStateAfterWrite(ctx, params.FilePath)
			msg := fmt.Sprintf("Lint check failed — edit rolled back.\n%s", result.Output)
			return NewTextErrorResponse(msg), nil
		}
	}
	refreshReadStateAfterWrite(ctx, params.FilePath)

	edits := myers.ComputeEdits(content, newContent)
	unified, _ := udiff.ToUnified(params.FilePath, params.FilePath, content, edits, 3)
	diffStr := unified

	return NewTextResponse(diffStr), nil
}
