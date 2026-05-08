package tools

// registerWebTools returns web search and fetch deferred tools.
// WebSearch is always registered so provider reloads can make it available dynamically.
// WebFetch is always registered (works with any provider).
func registerWebTools(deps ToolDeps) []BaseTool {
	if deps.WebRuntime == nil {
		return nil
	}

	return []BaseTool{
		NewWebSearchTool(deps.Perms, deps.WebRuntime),
		NewWebFetchTool(deps.Perms, deps.WebRuntime),
	}
}
