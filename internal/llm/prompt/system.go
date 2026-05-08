package prompt

// coderSystemPrompt has been migrated to modules/base.go and is now assembled
// by buildCoderPrompt() via the PromptBuilder.

const titleSystemPrompt = `Generate a short title (max 50 characters) for this conversation.
One line, no quotes, no colons. Just the title.`

const summarizerSystemPrompt = `You are a conversation summarizer. Do not call tools.

Output format:
<summary>
1. Primary Request and Intent
2. Key Technical Concepts
3. Files and Code Sections
4. Errors and Fixes
5. Problem Solving
6. All User Messages
7. Pending Tasks
8. Current Work
9. Optional Next Step
</summary>`
