package modules

// NewSubagentOrchestrationModule returns subagent orchestration guidance.
// When enabled, it switches to notification-driven TaskV2 semantics.
func NewSubagentOrchestrationModule(enabled bool) BaseModule {
	if enabled {
		return NewBaseModule("subagent-orchestration", subagentOrchestrationV2Prompt, 74)
	}
	return NewBaseModule("subagent-orchestration", subagentOrchestrationLegacyPrompt, 74)
}

const subagentOrchestrationLegacyPrompt = `## Subagent Orchestration

- Use TaskV2 for delegated work: create, send, stop, read, list.
- For long research, broad search, survey seed search, or tasks likely to need many ScholarSearch/WebSearch/KBSearch calls, create one or more Task workers with agent_type="research" and let them absorb raw results in child context.
- Parent context should receive only compressed candidate papers, evidence tables, convergence notes, and next steps from research workers; do not paste raw search dumps into the parent.
- Keep legacy convergence behavior: after background create, use Task list/read to collect terminal results.
- Worker prompts must be self-contained: task goal, constraints, files/scope, forbidden actions, output format.
- Main/coordinator must synthesize worker outputs directly; never outsource synthesis.
- Read-only tasks can run in parallel. Keep writable tasks scoped and avoid overlapping file edits.
- Use fresh verification after meaningful changes; do not reuse implementation worker context for verification.
- Do not read child transcript/debug_transcript in normal workflow. Use debug transcript only for incident debugging.`

const subagentOrchestrationV2Prompt = `## Subagent Orchestration V2

- Background Task results are injected as user-role <task-notification> messages in the next model request.
- After Task action=create returns the task view for independent background work, end the turn and wait for notification.
- For long research, broad search, survey seed search, or tasks likely to need many ScholarSearch/WebSearch/KBSearch calls, create one or more Task workers with agent_type="research" and let them absorb raw results in child context.
- Parent context should receive only compressed candidate papers, evidence tables, convergence notes, and next steps from research workers; do not paste raw search dumps into the parent.
- Do not sleep, poll Task list/read, or repeatedly query status just to wait for completion.
- Do not read child transcript/debug_transcript in normal workflow.
- Worker prompts must be self-contained: goal, context, file scope, constraints, acceptance output schema.
- Main/coordinator must synthesize all worker results in the parent session; never delegate synthesis back to workers.
- Read-only work may run in parallel. Writable workers require write_set and conflicting write sets must be serialized.
- Verification should be fresh and independent from implementation workers.`
