package tools

// registerScholarTools returns academic search and document writing deferred tools.
func registerScholarTools(deps ToolDeps) []BaseTool {
	result := []BaseTool{
		NewImageGenTool(deps.Perms),
		NewDiagramGenTool(deps.Perms),
		NewPaperValidateTool(deps.Perms),
		NewScholarSearchTool(deps.Perms),
		NewDocExportTool(deps.Perms),
	}

	if deps.AskBroker != nil {
		result = append(result, NewAskUserTool(deps.AskBroker))
	}

	return result
}
