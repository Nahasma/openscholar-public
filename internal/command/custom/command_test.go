package custom

import (
	"testing"

	"github.com/openscholar/openscholar/internal/command"
)

func TestCustomCommandExecute_EmitsRuntimeOverride(t *testing.T) {
	cmd := &CustomCommand{
		name:        "test",
		description: "test",
		body:        "run $ARGUMENTS",
		frontmatter: Frontmatter{
			AllowedTools:              []string{"View", "Bash"},
			Model:                     "claude-sonnet-4-20250514",
			DisableModelInvocationAlt: true,
		},
	}

	res := cmd.Execute(command.Context{Args: "now"})
	if res.Prompt != "run now" {
		t.Fatalf("prompt mismatch: got %q", res.Prompt)
	}
	if res.Runtime == nil {
		t.Fatalf("expected runtime override metadata")
	}
	if got := res.Runtime.Model; got != "claude-sonnet-4-20250514" {
		t.Fatalf("model mismatch: got %q", got)
	}
	if len(res.Runtime.AllowedTools) != 2 {
		t.Fatalf("allowed tools len mismatch: got %d", len(res.Runtime.AllowedTools))
	}
	if !res.Runtime.DisableModelInvocation {
		t.Fatalf("expected disable-model-invocation=true")
	}

	spec := cmd.Spec()
	if spec.Source != "custom" {
		t.Fatalf("source = %q, want custom", spec.Source)
	}
	if spec.ArgumentHint != "" {
		t.Fatalf("argument hint should default empty when not set")
	}
}
