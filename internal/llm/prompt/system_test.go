package prompt

import (
	"strings"
	"testing"
)

func TestSummarizerSystemPrompt_HasNineSectionsAndNoTools(t *testing.T) {
	if !strings.Contains(summarizerSystemPrompt, "Do not call tools") {
		t.Fatalf("expected no-tools instruction in summarizer prompt")
	}
	for _, s := range []string{
		"1. Primary Request and Intent",
		"2. Key Technical Concepts",
		"3. Files and Code Sections",
		"4. Errors and Fixes",
		"5. Problem Solving",
		"6. All User Messages",
		"7. Pending Tasks",
		"8. Current Work",
		"9. Optional Next Step",
	} {
		if !strings.Contains(summarizerSystemPrompt, s) {
			t.Fatalf("missing section %q", s)
		}
	}
}
