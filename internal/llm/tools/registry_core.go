package tools

// registerCoreTools returns the 6 core file-operation tools that are always
// sent to the LLM (not deferred).
func registerCoreTools(deps ToolDeps) []BaseTool {
	result := []BaseTool{
		NewViewTool(deps.Perms),
		NewEditTool(deps.Perms),
		NewWriteTool(deps.Perms),
		NewBashTool(deps.Perms),
		NewGlobTool(deps.Perms),
		NewGrepTool(deps.Perms),
	}
	if deps.Plans != nil {
		result = append(result, NewEnterPlanModeTool(deps.Perms, deps.Plans, deps.Sessions))
		if deps.PlanBroker != nil {
			result = append(result, NewExitPlanModeTool(deps.Perms, deps.Plans, deps.PlanBroker))
		}
	}
	return result
}
