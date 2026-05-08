---
name: "init_skill"
description: "Recognize reusable workflows and, with explicit user approval, create or refine SkillBank skills including first-run review."
category: "workflow"
tags: ["skillbank", "workflow", "reuse", "init-skill"]
version: 1
author: "system"
when_to_use: "Use when a workflow shows at least two reuse signals (repeatability, stable IO, multi-step/tool chain, or persistent user corrections) and the user has not already rejected skillization in this conversation."
allowed-tools: ["ToolSearch", "SkillQuery", "SkillManage", "AskUser", "RecordFeedback", "View", "Glob", "Grep"]
effort: "medium"
user-invocable: true
exposure: implicit
source: "builtin"
---
# Purpose
Turn reusable workflows into maintainable skills without surprise writes.

# Recognition
- Look for at least two signals before asking to skillize:
  - repeated or recurring workflow
  - stable inputs and outputs
  - multi-step flow across tools
  - user corrections that should become long-term rules

# Do Not Trigger
- One-off tasks.
- Sensitive/private procedures or secrets.
- Unstable workflows still being explored.
- Cases where user already rejected skill creation in this conversation.
- Research mode when skill writes are blocked.

# Confirmation Protocol
1. Ask for explicit user confirmation before any create/update/delete.
2. Keep questions minimal; gather only fields required to create or update safely.
3. If user declines, stop asking again for the same opportunity in this conversation.

# Duplicate Check (Required Before Create)
1. Run SkillQuery search by need and candidate names.
2. If duplicate or near-match exists, ask user to choose:
   - use existing skill,
   - update existing skill,
   - create a new distinct skill.
3. Only call SkillManage create after the duplicate decision is explicit.

# Create/Update Flow
1. Confirm write permission and mode.
2. Build a concise skill draft with:
   - name, description, when_to_use
   - allowed tools
   - workflow steps
   - input/output expectations
   - constraints and quality checks
   - post-run review checklist
3. Ask user to confirm the final draft intent.
4. Execute SkillManage create/update only after that confirmation.

# First Run
- After creation, run the workflow once in the current session using the skill instruction text (do not depend on immediate slash registration).

# Post-Run Skill Review
1. Review first-run execution for missing steps, brittle assumptions, and user corrections.
2. Present proposed skill changes.
3. Require explicit confirmation before SkillManage update.
4. If user declines update, optionally RecordFeedback for later evolution.
