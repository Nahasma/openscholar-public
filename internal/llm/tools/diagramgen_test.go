package tools

import (
	"strings"
	"testing"
)

func TestNormalizeAndLintD2_AutofixUniqueNestedRefAndStrokeDash(t *testing.T) {
	in := `Encoder: Encoder {
  MHA: Multi-Head Attention
  style: { stroke-dash: true }
}
Decoder: Decoder {
  Cross: Cross Attention
}
MHA -> Cross`

	out, warnings, errs := normalizeAndLintD2(in, true)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if !strings.Contains(out, "Encoder.MHA -> Decoder.Cross") {
		t.Fatalf("expected qualified nested refs, got:\n%s", out)
	}
	if !strings.Contains(out, "stroke-dash: 3") {
		t.Fatalf("expected stroke-dash normalization, got:\n%s", out)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected quality warnings")
	}
}

func TestNormalizeAndLintD2_StrictAmbiguousNestedRef(t *testing.T) {
	in := `A: GroupA {
  X: Node X
}
B: GroupB {
  X: Node X
}
X -> A`
	_, _, errs := normalizeAndLintD2(in, true)
	if len(errs) == 0 {
		t.Fatalf("expected ambiguity error in strict mode")
	}
}

func TestNormalizeAndLintD2_DoesNotDuplicateQualifiedRefs(t *testing.T) {
	in := `Encoder: Encoder {
  MHA: Multi-Head Attention
}
Decoder: Decoder {
  Cross: Cross Attention
}
Encoder.MHA -> Cross`

	out, _, errs := normalizeAndLintD2(in, true)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if strings.Contains(out, "Encoder.Encoder.MHA") {
		t.Fatalf("qualified source was duplicated:\n%s", out)
	}
	if !strings.Contains(out, "Encoder.MHA -> Decoder.Cross") {
		t.Fatalf("expected destination qualified only, got:\n%s", out)
	}
}

func TestNormalizeAndLintD2_InlineStyleDoesNotPopContainer(t *testing.T) {
	in := `Encoder: Encoder {
  MHA: Multi-Head Attention
  style: { stroke-dash: true }
  FFN: Feed Forward
}
MHA -> FFN`

	out, _, errs := normalizeAndLintD2(in, true)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if !strings.Contains(out, "Encoder.MHA -> Encoder.FFN") {
		t.Fatalf("expected both refs to stay under Encoder, got:\n%s", out)
	}
}

func TestNormalizeAndLintD2_QualifiesDeepNestedRefs(t *testing.T) {
	in := `Encoder: Encoder {
  Layer: Layer Block {
    MHA: Multi-Head Attention
  }
}
MHA -> Encoder`

	out, _, errs := normalizeAndLintD2(in, true)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if !strings.Contains(out, "Encoder.Layer.MHA -> Encoder") {
		t.Fatalf("expected fully qualified deep ref, got:\n%s", out)
	}
}

func TestDefaultThemeForDiagramStyle(t *testing.T) {
	if got := defaultThemeForDiagramStyle("paper"); got != 3 {
		t.Fatalf("paper default theme = %d, want 3", got)
	}
	if got := defaultThemeForDiagramStyle("none"); got != 0 {
		t.Fatalf("none default theme = %d, want 0", got)
	}
}
