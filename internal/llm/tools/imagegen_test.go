package tools

import (
	"strings"
	"testing"
)

func TestEnhanceImagePrompt_PaperDefault(t *testing.T) {
	p := enhanceImagePrompt("A conceptual diagram of model collaboration", "paper", "high")
	if !strings.Contains(strings.ToLower(p), "white background") {
		t.Fatalf("expected white background hint, got: %s", p)
	}
	if !strings.Contains(strings.ToLower(p), "low clutter") {
		t.Fatalf("expected low clutter hint, got: %s", p)
	}
	if !strings.Contains(strings.ToLower(p), "do not include any text") {
		t.Fatalf("expected no-text constraint, got: %s", p)
	}
}

func TestEnhanceImagePrompt_ShortTextDoesNotAddContradictoryNoText(t *testing.T) {
	p := enhanceImagePromptWithOptions("A badge with the word \"AI\"", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyShortText,
	})
	lower := strings.ToLower(p)
	if strings.Contains(lower, "do not include any text") {
		t.Fatalf("short_text should not add contradictory no-text constraint: %s", p)
	}
	if !strings.Contains(lower, "only the explicitly requested short text") {
		t.Fatalf("short_text should constrain extra words: %s", p)
	}
}

func TestEnhanceImagePrompt_AutoShortTextIntent(t *testing.T) {
	p := enhanceImagePromptWithOptions("A badge with the word \"AI\"", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyAuto,
	})
	lower := strings.ToLower(p)
	if strings.Contains(lower, "do not include any text") {
		t.Fatalf("auto short text should not add no-text: %s", p)
	}
	if !strings.Contains(lower, "only the explicitly requested short text") {
		t.Fatalf("auto short text should add short text constraint: %s", p)
	}
}

func TestEnhanceImagePrompt_AutoQuotedNonTextIntentStillForbidsText(t *testing.T) {
	for _, prompt := range []string{
		"A minimalist illustration of \"retrieval augmented generation\"",
		"A minimalist illustration of context \"retrieval augmented generation\"",
	} {
		p := enhanceImagePromptWithOptions(prompt, imagePromptOptions{
			StylePreset: "paper",
			Quality:     "high",
			TextPolicy:  imageTextPolicyAuto,
		})
		lower := strings.ToLower(p)
		if !strings.Contains(lower, "do not include any text") {
			t.Fatalf("quoted non-text phrase should still add no-text constraint: %s", p)
		}
		if strings.Contains(lower, "only the explicitly requested short text") {
			t.Fatalf("quoted non-text phrase should not be treated as short text: %s", p)
		}
	}
}

func TestEnhanceImagePrompt_AutoPreciseTextRoutesToOverlayBase(t *testing.T) {
	p := enhanceImagePromptWithOptions("A diagram with axis labels, legend, and formula E=mc^2", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyAuto,
	})
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "leave clean open space for later overlay labels") {
		t.Fatalf("precise text intent should route to overlay base prompt: %s", p)
	}
	if !strings.Contains(imageTextRouteWarning("A diagram with axis labels", imageTextPolicyAuto), "DiagramGen") {
		t.Fatalf("expected routing warning for precise text intent")
	}
}

func TestImageTextRouteWarningPreciseIntentCannotBeBypassedByPolicy(t *testing.T) {
	for _, policy := range []string{imageTextPolicyForbid, imageTextPolicyShortText, imageTextPolicyOverlayBase} {
		if got := imageTextRouteWarning("A plot with x-axis labels and a legend", policy); !strings.Contains(got, "DiagramGen") {
			t.Fatalf("policy %q should still warn for precise chart text, got %q", policy, got)
		}
	}
}

func TestEnhanceImagePrompt_NoLabelsDoesNotSuppressNoTextConstraint(t *testing.T) {
	p := enhanceImagePromptWithOptions("A poster with title \"AI\", no labels", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyForbid,
	})
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "do not include any text") {
		t.Fatalf("no labels alone should not suppress full no-text constraint: %s", p)
	}
}

func TestEnhanceImagePrompt_OverlayBaseNoLabelsStillLeavesOpenSpace(t *testing.T) {
	p := enhanceImagePromptWithOptions("A clean system mechanism illustration, no labels", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyOverlayBase,
	})
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "leave clean open space") {
		t.Fatalf("no labels alone should not suppress overlay spacing instruction: %s", p)
	}
}

func TestEnhanceImagePrompt_OverlayBaseNoTextStillLeavesOpenSpace(t *testing.T) {
	p := enhanceImagePromptWithOptions("A clean system mechanism illustration, no text, no labels", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyOverlayBase,
	})
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "leave clean open space") {
		t.Fatalf("overlay_base with explicit no-text should still add overlay spacing instruction: %s", p)
	}
}

func TestEnhanceImagePrompt_OverlayBaseLeavesSpaceForLabels(t *testing.T) {
	p := enhanceImagePromptWithOptions("A clean system mechanism illustration", imagePromptOptions{
		StylePreset: "paper",
		Quality:     "high",
		TextPolicy:  imageTextPolicyOverlayBase,
	})
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "do not include any text") || !strings.Contains(lower, "leave clean open space") {
		t.Fatalf("overlay_base should produce no-text base image guidance: %s", p)
	}
}

func TestEnhanceImagePrompt_RespectsExplicitIntent(t *testing.T) {
	p := enhanceImagePrompt("A photorealistic scene with dark background", "paper", "high")
	lower := strings.ToLower(p)
	if strings.Contains(lower, "white background") {
		t.Fatalf("should not force white background for explicit dark intent: %s", p)
	}
	if strings.Contains(lower, "clean scientific illustration style") {
		t.Fatalf("should not force scientific style for explicit photorealistic intent: %s", p)
	}
}

func TestResolveAndValidateImageSize(t *testing.T) {
	openai := &openaiDalleProvider{}
	if got := resolveImageSize(openai, "", "paper"); got != "1792x1024" {
		t.Fatalf("paper default size = %q, want 1792x1024", got)
	}
	if err := validateImageSize(openai, "800x800"); err == nil {
		t.Fatalf("expected size validation error")
	}
	if err := validateImageSize(openai, "1024x1024"); err != nil {
		t.Fatalf("expected valid size, got err: %v", err)
	}
}
