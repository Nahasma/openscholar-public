package tools

// registerResearchTools returns Research pipeline deferred tools.
func registerResearchTools(deps ToolDeps) []BaseTool {
	if deps.ResearchCtrl == nil {
		return nil
	}
	return []BaseTool{
		// Legacy compatibility wrapper — keeps "ResearchControl" tool name working.
		NewResearchControlTool(deps.ResearchCtrl, deps.CheckpointBroker, deps.Perms, deps.RunAgent, deps.Sessions, deps.Messages),
		// Narrow tools — preferred for new agent prompts.
		NewResearchPipelineTool(deps.ResearchCtrl, deps.CheckpointBroker, deps.Perms, deps.RunAgent, deps.Sessions, deps.Messages),
		NewResearchTaskTool(deps.ResearchCtrl),
		NewResearchMessageTool(deps.ResearchCtrl),
	}
}
