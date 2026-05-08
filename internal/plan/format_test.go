package plan

import "testing"

func TestWrapAndNormalize(t *testing.T) {
	raw := "## Plan\n- one\n- two\n"
	got := WrapProposed(raw)
	want := "<proposed_plan>\n## Plan\n- one\n- two\n</proposed_plan>"
	if got != want {
		t.Fatalf("WrapProposed mismatch:\n%s", got)
	}
	if body := NormalizeBody(got); body != "## Plan\n- one\n- two" {
		t.Fatalf("NormalizeBody mismatch: %q", body)
	}
}

func TestNormalizeApprovedEnvelope(t *testing.T) {
	raw := "<approved_plan>\nline 1\nline 2\n</approved_plan>"
	if body := NormalizeBody(raw); body != "line 1\nline 2" {
		t.Fatalf("NormalizeBody approved mismatch: %q", body)
	}
}

func TestWrapIsIdempotentForProposed(t *testing.T) {
	raw := "<proposed_plan>\nhello\n</proposed_plan>"
	got := WrapProposed(raw)
	if got != raw {
		t.Fatalf("double wrap detected: %q", got)
	}
}

func TestNormalizeOnlyOuterEnvelope(t *testing.T) {
	raw := "text with <proposed_plan>inline</proposed_plan> body"
	if body := NormalizeBody(raw); body != raw {
		t.Fatalf("unexpected normalization: %q", body)
	}
}
