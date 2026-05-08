package modules

// NewRebuttalModule returns the prompt module for rebuttal writing guidance.
// Priority 74: loads after cross_compare (73).
func NewRebuttalModule() BaseModule {
	return NewBaseModule("rebuttal", rebuttalPrompt, 74)
}

const rebuttalPrompt = `# Rebuttal Writing Guide

When the user needs to write a rebuttal or respond to reviewer comments:

## Input Formats
- Plain text pasted in chat
- PDF file (use View tool to read)
- .txt or .md file with reviewer comments

## Response Structure
For each reviewer, organize responses as:

\section*{Response to Reviewer 1}

\textbf{[R1.1] Summary of concern}

\textit{Reviewer wrote: "..."}

Response text here. Changes are marked in {\color{blue}blue} in the revised manuscript.

## Best Practices
- Address ALL points, even minor ones
- Lead with acknowledgment before counter-argument
- Reference specific sections/pages when describing changes
- Include any new experimental results as figures or tables
`
