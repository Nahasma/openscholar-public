package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/fileop"
	"github.com/openscholar/openscholar/internal/permission"
)

type grepTool struct{ permissions permission.Service }

type grepParams struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	Include       string `json:"include,omitempty"`
	Glob          string `json:"glob,omitempty"`
	Type          string `json:"type,omitempty"`
	OutputMode    string `json:"output_mode,omitempty"`
	HeadLimit     int    `json:"head_limit,omitempty"`
	Offset        int    `json:"offset,omitempty"`
	Context       int    `json:"context,omitempty"`
	BeforeContext int    `json:"before_context,omitempty"`
	AfterContext  int    `json:"after_context,omitempty"`
	IgnoreCase    bool   `json:"ignore_case,omitempty"`
	I             bool   `json:"i,omitempty"`
	Multiline     bool   `json:"multiline,omitempty"`
}

func NewGrepTool(perms permission.Service) BaseTool { return &grepTool{permissions: perms} }

func (t *grepTool) Info() ToolInfo {
	return ToolInfo{Name: "Grep", MaxResultBytes: 12 * 1024, Description: "Search file contents using a regular expression pattern. Returns matching files and lines.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string", "description": "Regular expression pattern to search for"}, "path": map[string]any{"type": "string", "description": "Directory to search in (defaults to current directory)"}, "include": map[string]any{"type": "string", "description": "File pattern to include (e.g., '*.tex', '*.go')"}, "glob": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}, "output_mode": map[string]any{"type": "string", "enum": []string{"content", "files_with_matches", "count"}}, "head_limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "context": map[string]any{"type": "integer"}, "before_context": map[string]any{"type": "integer"}, "after_context": map[string]any{"type": "integer"}, "ignore_case": map[string]any{"type": "boolean"}, "i": map[string]any{"type": "boolean"}, "multiline": map[string]any{"type": "boolean"}}, "required": []string{"pattern"}}, Required: []string{"pattern"}}
}

func (t *grepTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params grepParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	searchPath := strings.TrimSpace(params.Path)
	if searchPath == "" {
		searchPath = WorkspaceDir(ctx)
	}
	if !IsResearchMode(ctx) {
		if err := ValidateWorkspacePath(ctx, searchPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
		}
	}
	if IsResearchMode(ctx) {
		if err := ValidateResearchPath(ctx, searchPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Research sandbox: %v", err)), nil
		}
		if !filepath.IsAbs(searchPath) {
			searchPath = filepath.Join(ResearchWorkDir(ctx), searchPath)
		}
	}
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{SessionID: sessionID, ToolName: "Grep", Description: params.Pattern + " in " + searchPath, Action: "read", Path: searchPath})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}
	rp, err := fileop.ResolvePath(searchPath, WorkspaceDir(ctx))
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid path: %v", err)), nil
	}
	mode := strings.TrimSpace(params.OutputMode)
	if mode == "" {
		mode = "content"
	}
	res, err := fileop.RunSearch(fileop.SearchOptions{Pattern: params.Pattern, Path: rp.Abs, Glob: nonEmpty(params.Glob, params.Include), Include: params.Include, Type: params.Type, OutputMode: mode, HeadLimit: params.HeadLimit, Offset: params.Offset, ContextCombined: params.Context, ContextBefore: params.BeforeContext, ContextAfter: params.AfterContext, IgnoreCase: params.IgnoreCase || params.I, Multiline: params.Multiline})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Search error: %v", err)), nil
	}
	if len(res.Lines) == 0 && len(res.Paths) == 0 && res.Count == 0 {
		return NewTextResponse("No matches found"), nil
	}
	paths := res.Paths
	if len(paths) == 0 {
		m := make(map[string]struct{})
		for _, ln := range res.Lines {
			if idx := strings.Index(ln, ":"); idx > 0 {
				m[ln[:idx]] = struct{}{}
			}
		}
		for p := range m {
			paths = append(paths, p)
		}
	}
	notifyFileToolUsage(ctx, "Grep", paths)
	if mode == "files_with_matches" {
		content := "Found " + fmt.Sprintf("%d", len(res.Paths)) + " files\n" + strings.Join(res.Paths, "\n")
		if res.Truncated {
			content += "\n[search output truncated]"
		}
		return NewTextResponse(content), nil
	}
	content := strings.Join(res.Lines, "\n")
	if res.Truncated {
		content += "\n[search output truncated]"
	}
	if mode == "count" {
		return NewTextResponse(content), nil
	}
	return NewTextResponse(content), nil
}

func nonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
