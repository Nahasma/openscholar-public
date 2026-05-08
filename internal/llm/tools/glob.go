package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type globTool struct{ permissions permission.Service }

type globParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func NewGlobTool(perms permission.Service) BaseTool { return &globTool{permissions: perms} }

func (t *globTool) Info() ToolInfo {
	return ToolInfo{Name: "Glob", MaxResultBytes: 12 * 1024, Description: "Find files matching a glob pattern. Supports ** for recursive matching. Returns file paths sorted by modification time.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g., '**/*.tex', 'sections/*.tex')"}, "path": map[string]any{"type": "string", "description": "Directory to search in (defaults to current directory)"}}, "required": []string{"pattern"}}, Required: []string{"pattern"}}
}

func (t *globTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params globParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	if err := validateSearchPattern(params.Pattern); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid pattern: %v", err)), nil
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
		allowed := t.permissions.Request(permission.CreatePermissionRequest{SessionID: sessionID, ToolName: "Glob", Description: params.Pattern + " in " + searchPath, Action: "read", Path: searchPath})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}
	rp, err := fileop.ResolvePath(searchPath, WorkspaceDir(ctx))
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid path: %v", err)), nil
	}
	pattern := filepath.Join(rp.Abs, params.Pattern)
	matches, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Glob error: %v", err)), nil
	}
	type fi struct {
		path string
		mod  int64
	}
	files := make([]fi, 0, len(matches))
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil || st.IsDir() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(m)
		if err != nil {
			continue
		}
		if !fileop.InBoundary(resolved, rp.Resolved) {
			continue
		}
		files = append(files, fi{path: m, mod: st.ModTime().Unix()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod == files[j].mod {
			return files[i].path < files[j].path
		}
		return files[i].mod > files[j].mod
	})
	truncated := false
	if len(files) > 100 {
		files = files[:100]
		truncated = true
	}
	paths := make([]string, 0, len(files))
	var b strings.Builder
	for _, f := range files {
		rel, err := filepath.Rel(WorkspaceDir(ctx), f.path)
		out := f.path
		if err == nil && !strings.HasPrefix(rel, "..") {
			out = rel
		}
		paths = append(paths, f.path)
		b.WriteString(out)
		b.WriteByte('\n')
	}
	if len(files) == 0 {
		return NewTextResponse("No files found"), nil
	}
	if truncated {
		b.WriteString("... results truncated to 100 files\n")
	}
	notifyFileToolUsage(ctx, "Glob", paths)
	return NewTextResponse(b.String()), nil
}
