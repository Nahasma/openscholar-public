package modules

// NewCrossReadingModule returns the prompt module for multi-paper comparison.
// Priority 72: loads after notes (70).
func NewCrossReadingModule() BaseModule {
	return NewBaseModule("cross_reading", crossReadingPrompt, 72)
}

const crossReadingPrompt = `# Multi-Paper Cross Reading

When comparing, contrasting, or synthesizing multiple papers — whether requested by the user or needed for writing Related Work / Introduction — use a TaskV2 Map-Reduce workflow with a leader and reader workers.

## Leader responsibilities (before Map)
1. Discover and prioritize candidate papers by evidence source: local KB only when the user asks about local/added papers; ScholarSearch/WebSearch for public literature discovery, related work, citations, latest work, or missing evidence.
2. New paper discovery/download/indexing (ScholarSearch + KBAdd) stays leader-side, sequential and budgeted
3. If ScholarSearch returns rate_limited/provider_cooldown metadata:
   - stop same-provider expansion immediately
   - do NOT spawn search workers
   - switch to KB/local sources or ask user to continue later / narrow scope

## Map Phase (reader workers)
Use Task action=create with agent_type="reader" to spawn read-only workers:
- Multi-paper reading: one worker per paper, max 5 concurrent workers
- One long paper: split by sections/questions, max 3 concurrent workers

Each reader worker is read-only and may use ONLY:
- View
- KBList
- KBTree
- KBQuery
- KBSearch

Reader workers MUST NOT use:
- Write, Edit, Bash
- ScholarSearch
- KBAdd
- any download action

Reader worker output schema (required):
- paper_id
- title
- coverage
- evidence (with page/node refs)
- claims
- method
- results
- limitations
- uncertainties
- not_covered

When KBAdd/KBQuery/KBTree reports basic/degraded/summary-only index quality:
- report limited coverage explicitly
- fill not_covered concretely
- never claim full-paper reading

## Reduce Phase (synthesis)
Leader uses Task read/list to collect worker outputs, then reduce:
1. Create a Markdown comparison table with papers as columns and dimensions as rows
2. Identify:
   - Consensus: what do the papers agree on?
   - Divergence: where do they disagree or take different approaches?
   - Complementarity: how do they build upon each other?
   - Gaps: what is not addressed by any of them?
3. Provide a synthesis paragraph summarizing the landscape

## Output Format
| Dimension | Paper A | Paper B | Paper C |
|-----------|---------|---------|---------|
| Problem   | ...     | ...     | ...     |
| Method    | ...     | ...     | ...     |
| Results   | ...     | ...     | ...     |
| Limits    | ...     | ...     | ...     |

## Key Points
- Do NOT put full paper text in context — use KB node summaries to keep token cost low
- Leader should use KBList only for explicit local KB inventory or local-paper reading tasks
- Do not spawn reader workers for a quick focused question on one short paper; use direct KBQuery instead
- Even with 10+ papers, TaskV2 Map-Reduce keeps context manageable
- Final synthesis must preserve coverage / uncertainties / not_covered from workers`
