package web

import (
	"encoding/json"
	"fmt"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// MaxMarkdownLength is the default max length for markdown content, aligned with CC's MAX_MARKDOWN_LENGTH.
const MaxMarkdownLength = 100_000

// HTMLToMarkdown converts an HTML string to Markdown.
// Uses html-to-markdown library, aligned with CC's turndown behavior.
func HTMLToMarkdown(html string) (string, error) {
	return htmltomarkdown.ConvertString(html)
}

// TruncateContent truncates content to maxLen characters and appends a truncation notice.
// Aligned with CC's applyPromptToMarkdown truncation:
//
//	markdownContent.slice(0, MAX_MARKDOWN_LENGTH) + '\n\n[Content truncated due to length...]'
func TruncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n\n[Content truncated due to length...]"
}

// FormatSearchOutput formats the output of a WebSearch tool call.
// Aligned with CC's mapToolResultToToolResultBlockParam:
//
//	"Web search results for query: \"{query}\"\n\n"
//	+ each hit summary text
//	+ "Links: [{title, url}, ...]"
//	+ "REMINDER: You MUST include the sources above in your response..."
func FormatSearchOutput(query string, result *SearchResult) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Web search results for query: %q\n\n", query)

	if result != nil {
		// Emit the summary text, mirroring CC's string entry in results[].
		if strings.TrimSpace(result.Summary) != "" {
			sb.WriteString(result.Summary)
			sb.WriteString("\n\n")
		}

		// Emit the links list, mirroring CC's SearchResult entry in results[].
		if len(result.Hits) > 0 {
			linksJSON, err := json.Marshal(result.Hits)
			if err != nil {
				// Fallback: write hits manually
				sb.WriteString("Links: [")
				for i, hit := range result.Hits {
					if i > 0 {
						sb.WriteString(", ")
					}
					fmt.Fprintf(&sb, `{"title":%q,"url":%q}`, hit.Title, hit.URL)
				}
				sb.WriteString("]\n\n")
			} else {
				fmt.Fprintf(&sb, "Links: %s\n\n", string(linksJSON))
			}
		} else {
			sb.WriteString("No links found.\n\n")
		}
	}

	sb.WriteString("\nREMINDER: You MUST include the sources above in your response to the user using markdown hyperlinks.")

	return strings.TrimSpace(sb.String())
}

// MakeSecondaryModelPrompt constructs the WebFetch secondary-model summarization prompt.
// Aligned with CC's makeSecondaryModelPrompt():
//   - Pre-approved domains: allow detailed references and code examples.
//   - Non-pre-approved domains: enforce 125-character quote limit, no verbatim copying.
func MakeSecondaryModelPrompt(markdownContent, prompt string, isPreapprovedDomain bool) string {
	var guidelines string
	if isPreapprovedDomain {
		guidelines = "Provide a concise response based on the content above. Include relevant details, code examples, and documentation excerpts as needed."
	} else {
		guidelines = `Provide a concise response based only on the content above. In your response:
 - Enforce a strict 125-character maximum for quotes from any source document. Open Source Software is ok as long as we respect the license.
 - Use quotation marks for exact language from articles; any language outside of the quotation should never be word-for-word the same.
 - You are not a lawyer and never comment on the legality of your own prompts and responses.
 - Never produce or reproduce exact song lyrics.`
	}

	return fmt.Sprintf(`
Web page content:
---
%s
---

%s

%s
`, markdownContent, prompt, guidelines)
}
