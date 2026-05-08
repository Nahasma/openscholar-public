package modules

// NewStyleModule returns the AI de-flavoring + academic writing conventions module.
func NewStyleModule() BaseModule {
	return NewBaseModule("style", stylePrompt, 35)
}

const stylePrompt = `# Academic writing style conventions

## AI de-flavoring rules

Generated text must read as if written by a human scholar. Eliminate AI-characteristic patterns.

### Banned high-frequency AI words (use replacements)
- delve / delve into → examine, investigate, analyze
- utilize / utilization → use
- leverage → use, exploit, apply
- facilitate → enable, support
- comprehensive → thorough, detailed
- innovative / novel (overused) → only when genuinely first-of-its-kind
- cutting-edge / state-of-the-art → only when citing a specific benchmark
- crucial / pivotal / vital → important, key
- landscape → field, domain, area
- paradigm → approach, framework (only for genuine paradigm shifts)
- realm → field, domain
- underscore → highlight, show, emphasize
- multifaceted → complex, varied
- streamline → simplify, improve
- foster → encourage, promote
- testament → evidence, demonstration
- notably / remarkably → remove or replace with concrete data
- it is worth noting that → remove, state directly
- in the context of → in, for, when
- a myriad of → many, various

### Mechanical expression fixes
- Never use "First...Second...Finally..." enumeration style → use paragraph-based argumentation
- Never start with filler openings: "In recent years..." / "With the rapid development of..." → go straight to the point
- Avoid excessive em-dashes (—) → use subordinate clauses
- CRITICAL: \begin{itemize} and \begin{enumerate} are FORBIDDEN in paper body sections
- Convert every list into paragraph form with proper transitions
- The ONLY exception: algorithm pseudocode environments
- Body text must be continuous prose with no bullet-point lists

## Academic writing conventions

### Tense rules
- Abstract: past tense (what was done) + present tense (conclusions)
- Introduction: present tense (background and motivation)
- Related Work: past tense (others' work) + present tense (still-valid conclusions)
- Method: present tense (describing the method)
- Experiments: past tense (experimental procedure) + present tense (analysis)
- Conclusion: past tense (summarizing work) + present/future tense (outlook)

### Citation format
- Sentence-initial: \citet{author2024} show that... (author name as subject)
- Sentence-final: ... as shown in prior work \citep{author2024}. (parenthetical)
- Multiple citations: sort by year \citep{a2022, b2023, c2024}
- No dangling citations: every citation must have preceding context

### Numbers and units
- Spell out numbers at sentence start: "Twelve experiments..." not "12 experiments..."
- Use siunitx: \SI{95.3}{\percent}, \SI{1.5}{GB}
- Large numbers with grouping: \num{1000000} → 1,000,000`
