package builtin

import (
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/command"
)

func TestKBRawFirstCommandPrompts(t *testing.T) {
	if !strings.Contains((&kbAddCmd{}).Description(), "raw-first") {
		t.Fatalf("kb-add description should mention raw-first")
	}
	treeRes := (&kbTreeCmd{}).Execute(command.Context{Args: "paper-1 pages"})
	if !strings.Contains(treeRes.Prompt, "view=pages") {
		t.Fatalf("kb-tree prompt should include view, got: %s", treeRes.Prompt)
	}
	askRes := (&askPaperCmd{}).Execute(command.Context{Args: "paper-1 \"question\""})
	if !strings.Contains(askRes.Prompt, "QueryPaper route") {
		t.Fatalf("ask-paper prompt should mention QueryPaper route, got: %s", askRes.Prompt)
	}
	if !strings.Contains((&kbHealthCmd{}).Execute(command.Context{}).Prompt, "KBHealth") {
		t.Fatal("kb-health should route to KBHealth tool")
	}
}
