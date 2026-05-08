package tools

// registerKnowledgeTools returns KB, Memory and SkillBank deferred tools.
func registerKnowledgeTools(deps ToolDeps) []BaseTool {
	var result []BaseTool

	if deps.KBs != nil && deps.KBs.KB != nil {
		result = append(result,
			NewKBAddTool(deps.KBs.KB, deps.KBs.Indexer),
			NewKBListTool(deps.KBs.KB),
			NewKBTreeTool(deps.KBs.KB),
		)
		callLLM := deps.CallLLM
		if callLLM == nil {
			callLLM = deps.KBs.CallLLM
		}
		memService := deps.MemService
		if memService == nil {
			memService = deps.KBs.MemService
		}
		if callLLM != nil {
			result = append(result,
				NewKBQueryTool(deps.KBs.KB, callLLM, memService),
				NewKBSearchTool(deps.KBs.KB, callLLM, memService),
				NewKBHealthTool(deps.KBs.KB),
				NewKBRepairTool(deps.KBs.KB),
				NewKBReindexTool(deps.KBs.KB),
			)
		}
	}

	if deps.Skills != nil {
		result = append(result,
			NewSkillQueryTool(deps.Skills),
			NewInvokeSkillTool(deps.Skills, deps.Perms, deps.Sessions, deps.Messages, deps.RunAgent, deps.KBs),
			NewSkillManageTool(deps.Skills),
		)
	}

	if deps.EvoService != nil {
		result = append(result, NewRecordFeedbackTool(deps.EvoService))
	}

	return result
}
