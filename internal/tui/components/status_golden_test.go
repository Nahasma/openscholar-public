package components

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderStatusBar_Golden(t *testing.T) {
	cases := []struct {
		name         string
		mode         string
		isProcessing bool
		hasInput     bool
		ctrlCPending bool
		width        int
		contextPct   int
	}{
		// Idle states
		{"status_idle_80", "", false, false, false, 80, 0},
		{"status_idle_ctx50_100", "", false, false, false, 100, 50},
		{"status_idle_ctx75_100", "", false, false, false, 100, 75},
		{"status_idle_ctx90_100", "", false, false, false, 100, 90},

		// Mode indicators
		{"status_auto_100", "auto", false, false, false, 100, 50},
		{"status_research_100", "research", false, false, false, 100, 50},
		{"status_default_input_100", "", false, true, false, 100, 50},

		// Processing
		{"status_processing_100", "", true, false, false, 100, 60},
		{"status_auto_processing_120", "auto", true, false, false, 120, 45},

		// Ctrl+C pending
		{"status_ctrlc_80", "", false, false, true, 80, 0},
		{"status_ctrlc_120", "", true, true, true, 120, 90},

		// Width variations
		{"status_idle_60", "", false, false, false, 60, 30},
		{"status_auto_80", "auto", false, true, false, 80, 85},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderBottomStatusLine(tc.mode, tc.isProcessing, tc.hasInput, tc.ctrlCPending, tc.width, tc.contextPct)
			checkGolden(t, tc.name, got)
		})
	}
}

func TestRenderInputArea_Golden(t *testing.T) {
	cases := []struct {
		name    string
		width   int
		content string
	}{
		{"input_empty_80", 80, ""},
		{"input_empty_120", 120, ""},
		{"input_short_80", 80, "Hello world"},
		{"input_multiline_100", 100, "First line\nSecond line\nThird line"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewInputModel()
			m.SetWidth(tc.width)
			if tc.content != "" {
				m.SetValue(tc.content)
			}
			got := m.View()
			checkGolden(t, tc.name, got)
		})
	}
}

func TestRenderInputFooter_Golden(t *testing.T) {
	cases := []struct {
		name   string
		params InputFooterParams
	}{
		{
			name: "input_footer_idle_100",
			params: InputFooterParams{
				Width: 100,
			},
		},
		{
			name: "input_footer_status_100",
			params: InputFooterParams{
				HasText: true,
				Notice: TransientNotice{
					Text: "Copied to clipboard ✓",
					Kind: NoticeSuccess,
				},
				Width: 100,
			},
		},
		{
			name: "input_footer_processing_80",
			params: InputFooterParams{
				IsProcessing: true,
				Width:        80,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderInputFooter(tc.params)
			checkGolden(t, tc.name, got)
		})
	}
}

func TestRenderInputFooter_OmitsFixedSessionMetadata(t *testing.T) {
	got := stripANSI(RenderInputFooter(InputFooterParams{
		Notice: TransientNotice{
			Text: "This is a deliberately long footer notice that should yield to the fixed right-side metadata",
			Kind: NoticeInfo,
		},
		Width: 52,
	}))

	if strings.Contains(got, "claude-3-7-sonnet") || strings.Contains(got, "research") {
		t.Fatalf("footer should not render fixed session metadata: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("footer notice should truncate within its own row: %q", got)
	}
}

func TestRenderInputFooter_DoesNotOwnEnterEscHints(t *testing.T) {
	plain := stripANSI(RenderInputFooter(InputFooterParams{
		HasText:      true,
		IsProcessing: true,
		Width:        100,
	}))
	if strings.Contains(plain, "enter to send") || strings.Contains(plain, "esc to interrupt") {
		t.Fatalf("input footer should not own enter/esc hints, got %q", plain)
	}
}

func TestInputFooterParamsExcludesSessionMetadataFields(t *testing.T) {
	typ := reflect.TypeOf(InputFooterParams{})
	for _, field := range []string{"Mode", "ModelName"} {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatalf("InputFooterParams should not expose session metadata field %s", field)
		}
	}
}

func TestRenderEnhancedStatusLine_PinsContextToRightEdge(t *testing.T) {
	got := stripANSI(RenderEnhancedStatusLine("", false, false, false, StatusMetrics{
		ContextPct:       7,
		PromptTokens:     1200,
		CompletionTokens: 752,
		CostUSD:          0.123,
	}, 80))

	trimmed := strings.TrimRight(got, " ")
	if !strings.HasSuffix(trimmed, "CTX 7%") {
		t.Fatalf("CTX should be the right-most status metric: %q", got)
	}
	if strings.Index(trimmed, "IN 1.2k OUT 752") > strings.Index(trimmed, "CTX 7%") {
		t.Fatalf("token metrics should stay left of CTX: %q", got)
	}
}
