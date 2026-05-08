package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestRenderConfigWizardDialog_MasksSensitiveProviderKeyInput(t *testing.T) {
	raw := "sk-secret-123"
	step := &config.CWStep{
		Type:        config.CWStepProviderKey,
		Label:       "配置 OpenAI 的 API Key",
		Placeholder: config.ProviderKeyPlaceholder(models.ProviderOpenAI),
		InputKind:   config.CWInputKindSensitive,
		Provider:    models.ProviderOpenAI,
	}

	got := RenderConfigWizardDialog(step, 1, 4, "Provider 配置", 0, raw, len([]rune(raw)), nil, 80)
	if strings.Contains(got, raw) {
		t.Fatalf("rendered dialog contains raw secret: %q", raw)
	}
	if !strings.Contains(got, strings.Repeat("*", len([]rune(raw)))) {
		t.Fatalf("rendered dialog did not contain masked secret of matching length")
	}
}

func TestRenderConfigWizardDialog_ProviderMenuTable(t *testing.T) {
	step := &config.CWStep{
		Type:  config.CWStepProviderMenu,
		Label: "选择要配置的 Provider",
		Options: []config.CWOption{
			{
				Label: "✓ OpenAI ****1234",
				Value: "openai",
				Display: &config.CWOptionDisplay{
					Primary:   "✓ OpenAI",
					Secondary: "****1234",
				},
			},
			{
				Label: "✗ Anthropic 未配置",
				Value: "anthropic",
				Display: &config.CWOptionDisplay{
					Primary:   "✗ Anthropic",
					Secondary: "未配置",
				},
			},
			{Label: "▶ 继续", Value: "continue"},
		},
	}

	got := RenderConfigWizardDialog(step, 1, 4, "Provider 配置", 0, "", 0, nil, 80)
	for _, want := range []string{"Provider", "API Key", "┌", "┐", "└", "┘", "▶ 继续"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output:\n%s", want, got)
		}
	}
	if strings.Index(got, "▶ 继续") < strings.Index(got, "└") {
		t.Fatalf("continue should render below table:\n%s", got)
	}
}

func TestRenderConfigWizardDialog_ProviderMenuNarrowWidth(t *testing.T) {
	step := &config.CWStep{
		Type:  config.CWStepProviderMenu,
		Label: "选择要配置的 Provider",
		Options: []config.CWOption{
			{
				Label: "✓ OpenAI",
				Value: "openai",
				Display: &config.CWOptionDisplay{
					Primary:   "✓ OpenAI",
					Secondary: "https://very-long-provider-url.example.com/v1/chat/completions",
				},
			},
			{Label: "▶ 继续", Value: "continue"},
		},
	}

	got := RenderConfigWizardDialog(step, 1, 4, "Provider 配置", 0, "", 0, nil, 36)
	if !strings.Contains(got, "Provider") || !strings.Contains(got, "API Key") {
		t.Fatalf("table headers missing in narrow output:\n%s", got)
	}
	plain := stripANSI(got)
	var borderWidth int
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "┌") {
			borderWidth = lipgloss.Width(line)
			break
		}
	}
	if borderWidth == 0 {
		t.Fatalf("table border missing:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if !strings.ContainsAny(line, "│┌├└") {
			continue
		}
		if gotWidth := lipgloss.Width(line); gotWidth > borderWidth {
			t.Fatalf("table line width = %d, want <= border width %d: %q\n%s", gotWidth, borderWidth, line, plain)
		}
	}
}

func TestRenderConfigWizardDialog_ZeroWidthDoesNotPanic(t *testing.T) {
	for _, step := range []*config.CWStep{
		{
			Type:  config.CWStepProviderMenu,
			Label: "选择要配置的 Provider",
			Options: []config.CWOption{
				{Label: "OpenAI", Value: "openai"},
				{Label: "▶ 继续", Value: "continue"},
			},
		},
		{
			Type:        config.CWStepProviderKey,
			Label:       "配置 OpenAI 的 API Key",
			Placeholder: config.ProviderKeyPlaceholder(models.ProviderOpenAI),
			InputKind:   config.CWInputKindSensitive,
			Provider:    models.ProviderOpenAI,
		},
	} {
		got := RenderConfigWizardDialog(step, 1, 4, "Provider 配置", 0, "sk-test", 2, nil, 0)
		if strings.TrimSpace(stripANSI(got)) == "" {
			t.Fatalf("zero-width config dialog rendered empty output for step type %v", step.Type)
		}
	}
}
