package modules

// NewNotesModule returns the prompt module for paper note management.
// Priority 70: loads after self_review (68).
func NewNotesModule() BaseModule {
	return NewBaseModule("notes", notesPrompt, 70)
}

const notesPrompt = `# Paper Notes System

You can help the user manage research notes for papers they read. Notes are stored as plain Markdown files.

## Note Storage
- Save notes in a location that makes sense for the user's context:
  - If the user specifies a path, use it
  - If reading a file from a specific directory, save notes nearby (e.g., a notes/ subdirectory next to the source file)
  - Only fall back to notes/ in the working directory if no better location can be inferred
- Use the Write tool to create a new note file
- Use the View tool to read existing notes
- Use the Edit tool to append or modify notes

## Note Format
Each note file follows this structure:

` + "```" + `markdown
---
title: "Paper Title"
authors: ["Author1", "Author2"]
year: 2025
venue: "NeurIPS"
tags: ["tag1", "tag2"]
---

## Key Contributions
(Main contributions of the paper)

## Method
(Technical approach and methodology)

## Experiments & Results
(Key experimental findings)

## Limitations
(Acknowledged or identified limitations)

## Personal Thoughts
(User's reflections, connections to other work)
` + "```" + `

## Workflow
1. After reading or indexing a paper, AUTOMATICALLY create a note file with key findings
2. When revisiting a paper, read existing notes first and build upon them
3. Use the Glob tool to find existing notes in the relevant directory
4. Focus on insights, connections to other papers, and open questions — not just restating the abstract
5. Notes are your research memory — the more you write, the better your future work becomes`
