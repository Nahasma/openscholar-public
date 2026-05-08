package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
	"github.com/openscholar/openscholar/internal/permission"
)

type writeTool struct {
	permissions permission.Service
}

type writeParams struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func NewWriteTool(perms permission.Service) BaseTool {
	return &writeTool{permissions: perms}
}

func (t *writeTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "Write",
		Description: "Creates or overwrites a file with the given content.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file",
				},
			},
			"required": []string{"file_path", "content"},
		},
		Required: []string{"file_path", "content"},
	}
}

func (t *writeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params writeParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.FilePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
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

	// Permission check: Write is a write operation
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "Write",
			Description: params.FilePath,
			Action:      "write",
			Path:        params.FilePath,
			Params:      params,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	if err := os.MkdirAll(filepath.Dir(params.FilePath), 0o755); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to create directory: %v", err)), nil
	}

	// Capture pre-existing content for lint rollback (nil means file did not exist)
	var existingContent []byte
	if existing, err := os.ReadFile(params.FilePath); err == nil {
		existingContent = existing
		if err := enforceStaleGuard(ctx, params.FilePath, existing); err != nil {
			return NewTextErrorResponse(err.Error()), nil
		}
	}

	// Phase 5: capture file snapshot before writing
	captureCheckpointBeforeWrite(ctx, params.FilePath)

	if err := fileop.WriteFileAtomic(params.FilePath, []byte(params.Content), 0o644); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to write file: %v", err)), nil
	}
	notifyFileToolUsage(ctx, "Write", []string{params.FilePath})

	// Lint guard: run post-write checks if enabled
	if cfg := config.Get(); cfg != nil && cfg.Harness.LintGuardEnabled {
		guard := NewLintGuard(cfg.Harness.LintGuardLatex, cfg.Harness.LintGuardGo)
		if result := guard.Check(params.FilePath); result != nil && !result.Passed {
			// Roll back: restore original content or remove the newly created file
			if existingContent != nil {
				_ = fileop.WriteFileAtomic(params.FilePath, existingContent, 0o644)
			} else {
				_ = os.Remove(params.FilePath)
			}
			refreshReadStateAfterWrite(ctx, params.FilePath)
			msg := fmt.Sprintf("Lint check failed — write rolled back.\n%s", result.Output)
			return NewTextErrorResponse(msg), nil
		}
	}
	refreshReadStateAfterWrite(ctx, params.FilePath)

	resp := fmt.Sprintf("File written: %s (%d bytes)", params.FilePath, len(params.Content))

	// Output guardrail: validate reading report quality for notes/*.md
	if isReadingReport(params.FilePath) {
		if diag := validateReadingReport(params.Content); diag != "" {
			resp += "\n\n⚠ Report quality check:\n" + diag
		}
	}

	return NewTextResponse(resp), nil
}

func staleGuardMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("OPENSCHOLAR_FILE_STALE_GUARD")))
	switch mode {
	case "off", "warn", "enforce":
		return mode
	default:
		return "enforce"
	}
}

func enforceStaleGuard(ctx context.Context, path string, current []byte) error {
	mode := staleGuardMode()
	if mode == "off" {
		return nil
	}
	rs, _ := ctx.Value(ReadStateContextKey).(*fileop.ReadState)
	if rs == nil {
		if mode == "warn" {
			return nil
		}
		return fmt.Errorf("stale guard: view the file before editing/writing existing files")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	rec, ok := rs.Get(path, 0, 2000)
	if !ok || !rec.EditEligible {
		if mode == "warn" {
			return nil
		}
		return fmt.Errorf("stale guard: view the file before editing/writing existing files")
	}
	currentHash := fileop.HashContent(current)
	if !st.ModTime().Equal(rec.MTime) || st.Size() != rec.Size || rec.ContentHash != currentHash {
		if mode == "warn" {
			return nil
		}
		return fmt.Errorf("stale guard: file changed since last view, please re-run View before editing/writing")
	}
	return nil
}

func refreshReadStateAfterWrite(ctx context.Context, path string) {
	rs, _ := ctx.Value(ReadStateContextKey).(*fileop.ReadState)
	if rs == nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	lineEnding := "lf"
	if strings.Contains(string(data), "\r\n") {
		lineEnding = "crlf"
	}
	encoding := "utf-8"
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		encoding = "utf-8-bom"
	}
	rs.Put(fileop.FileReadRecord{CanonicalPath: path, MTime: st.ModTime(), Size: st.Size(), Offset: 0, Limit: 2000, ContentHash: fileop.HashContent(data), EditEligible: true, LineEnding: lineEnding, Encoding: encoding})
}

// isReadingReport checks if the file path looks like a reading report.
func isReadingReport(path string) bool {
	dir := filepath.Base(filepath.Dir(path))
	return dir == "notes" && strings.HasSuffix(path, ".md")
}

// Page reference patterns
var (
	pageRefPattern   = regexp.MustCompile(`(?i)(第\d+页|p\.\s*\d+|page\s+\d+|pp?\.\s*\d+)`)
	figureRefPattern = regexp.MustCompile(`(?i)(figure\s+\d+|fig\.\s*\d+|table\s+\d+|图\s*\d+|表\s*\d+)`)
)

// validateReadingReport runs rule-based quality checks on a reading report.
// Returns diagnostic string (empty if all checks pass).
func validateReadingReport(content string) string {
	var issues []string

	// Check 1: Page references
	if !pageRefPattern.MatchString(content) {
		issues = append(issues, "- No page references found. Consider adding specific page citations (e.g., \"Page 3\", \"第5页\") to ground the report in the source document.")
	}

	// Check 2: Figure/Table references
	if !figureRefPattern.MatchString(content) {
		issues = append(issues, "- No Figure/Table references found. Consider citing specific figures or tables from the paper (e.g., \"Figure 2\", \"Table 1\").")
	}

	// Check 3: Minimum length (rough proxy for depth)
	lines := strings.Count(content, "\n")
	if lines < 30 {
		issues = append(issues, fmt.Sprintf("- Report is relatively short (%d lines). Consider adding more detailed analysis.", lines))
	}

	if len(issues) == 0 {
		return ""
	}
	return strings.Join(issues, "\n")
}
