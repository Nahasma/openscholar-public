package tools

// registerDocxTools returns DOCX-related deferred tools.
func registerDocxTools(deps ToolDeps) []BaseTool {
	return []BaseTool{
		NewDocxPatchTool(deps.Perms),
		NewDocxEditTool(deps.Perms),
		NewDocxValidateTool(deps.Perms),
	}
}
