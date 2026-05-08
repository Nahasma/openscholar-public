package tools

import (
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/tools/codeagent"
)

// registerIntegrationTools builds the CodeAgent registry and returns its tool.
func registerIntegrationTools(deps ToolDeps) []BaseTool {
	codeAgentReg := codeagent.NewRegistry()
	if cfg := config.Get(); cfg != nil && cfg.CodeAgent != nil {
		if cfg.CodeAgent.Preferred != "" {
			codeAgentReg.SetPreferred(cfg.CodeAgent.Preferred)
		}
		for name, pcfg := range cfg.CodeAgent.Providers {
			codeAgentReg.SetProviderConfig(name, codeagent.ProviderConfig{
				AllowedTools: pcfg.AllowedTools,
				MaxTurns:     pcfg.MaxTurns,
			})
		}
	}
	if deps.MCPCaller != nil {
		codeAgentReg.RegisterMCPProviders(deps.MCPCaller)
	}
	return []BaseTool{NewCodeAgentTool(codeAgentReg)}
}
