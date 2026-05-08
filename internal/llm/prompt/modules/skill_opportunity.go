package modules

const skillOpportunityContent = `## Skill Opportunity Detection

Proactively notice reusable workflow opportunities, even when the user did not ask to create a skill.

Only propose skillization when at least two signals are present:
1. The workflow is repeated or explicitly recurring.
2. Steps, inputs, and outputs are stable enough to template.
3. The flow spans multiple tools or has non-trivial sequencing.
4. The user provides corrections that should become a standing rule.

Do not propose in one-off tasks, sensitive/private procedures, unstable workflows, or after the user has rejected this opportunity in the current conversation.

When skillization is appropriate:
1. Use ToolSearch first if needed to activate SkillQuery, SkillManage, AskUser, and RecordFeedback.
2. Ask for explicit user confirmation before any create/update/delete action.
3. Before create, run SkillQuery duplicate search and prefer reuse/update when an exact or near match exists.
4. If user confirms create/update, collect only minimal required details and apply SkillManage.
5. After first run using the new/updated skill, perform a post-run skill review:
   - summarize gaps or brittle steps,
   - show proposed changes,
   - require explicit confirmation again before SkillManage update,
   - or record the feedback via RecordFeedback when user declines updates.

Research mode write block still applies: do not write skills when skill modification is blocked by mode.`

func NewSkillOpportunityModule() BaseModule {
	return NewBaseModule("skill_opportunity", skillOpportunityContent, 9)
}
