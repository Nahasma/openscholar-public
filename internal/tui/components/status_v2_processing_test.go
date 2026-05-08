package components

import (
	"strings"
	"testing"
)

func TestShouldRenderBottomProcessing(t *testing.T) {
	tests := []struct {
		name         string
		isProcessing bool
		phase        uint8
		width        int
		want         bool
	}{
		{name: "normal_width_queued", isProcessing: true, phase: PhaseToolQueued, width: 100, want: false},
		{name: "normal_width_running", isProcessing: true, phase: PhaseToolRunning, width: 80, want: false},
		{name: "narrow_width_running", isProcessing: true, phase: PhaseToolRunning, width: 60, want: false},
		{name: "ultra_narrow_width_running", isProcessing: true, phase: PhaseToolRunning, width: 24, want: false},
		{name: "thinking_never_mirrors", isProcessing: true, phase: PhaseThinking, width: 60, want: false},
		{name: "not_processing", isProcessing: false, phase: PhaseToolQueued, width: 60, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRenderBottomProcessing(tt.isProcessing, tt.phase, tt.width); got != tt.want {
				t.Fatalf("ShouldRenderBottomProcessing(...)=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderStatusV2_ProcessingMirrorOwnershipAndFallback(t *testing.T) {
	t.Run("normal_width_processing_owned_by_progress_rail", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:      true,
			ProcessingPhase:   PhaseToolQueued,
			ProcessingVerb:    "Queued tools",
			ProcessingElapsed: 6,
			Width:             100,
		})
		plain := stripANSI(got)
		if strings.Contains(plain, "Queued tools") {
			t.Fatalf("status=%q, processing detail should belong to progress rail", plain)
		}
	})

	t.Run("narrow_width_processing_still_omits_status_detail", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:      true,
			ProcessingPhase:   PhaseToolQueued,
			ProcessingVerb:    "Queued tools",
			ProcessingElapsed: 6,
			Width:             60,
		})
		if strings.TrimSpace(got) == "" {
			t.Fatal("status should still render stable metadata on narrow width")
		}
		if strings.Contains(got, "Queued tools") {
			t.Fatalf("status=%q, queued label should belong to progress rail", got)
		}
	})

	t.Run("ultra_narrow_width_processing_still_omits_status_detail", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:      true,
			ProcessingPhase:   PhaseToolQueued,
			ProcessingVerb:    "Queued tools",
			ProcessingElapsed: 6,
			Width:             24,
		})
		if strings.TrimSpace(got) == "" {
			t.Fatal("status should still render stable metadata on ultra-narrow width")
		}
		if strings.Contains(got, "Queued tools") {
			t.Fatalf("status=%q, queued label should belong to progress rail", got)
		}
	})

	t.Run("ctrlc_pending_always_visible", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:    true,
			ProcessingPhase: PhaseToolRunning,
			CtrlCPending:    true,
			Width:           100,
		})
		if !strings.Contains(got, "Press Ctrl+C again to exit") {
			t.Fatalf("status=%q, want ctrl+c pending hint", got)
		}
	})

	t.Run("overlay_owns_bottom_row_over_processing", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:      true,
			ProcessingPhase:   PhaseToolRunning,
			ProcessingVerb:    "processing",
			ProcessingElapsed: 2,
			OverlayName:       "Help",
			Width:             60,
		})
		if !strings.Contains(got, "[Help]") {
			t.Fatalf("status=%q, want overlay label", got)
		}
		if strings.Contains(got, "processing") {
			t.Fatalf("status=%q, processing should not preempt overlay", got)
		}
	})

	t.Run("vim_owns_bottom_row_over_processing", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			IsProcessing:      true,
			ProcessingPhase:   PhaseStreaming,
			ProcessingVerb:    "streaming",
			ProcessingElapsed: 2,
			VimMode:           "normal",
			Width:             60,
		})
		if !strings.Contains(got, "NORMAL") {
			t.Fatalf("status=%q, want vim mode", got)
		}
		if strings.Contains(strings.ToLower(got), "streaming") {
			t.Fatalf("status=%q, processing should not preempt vim", got)
		}
	})

	t.Run("mode_overrides_model_path", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			Mode:      "auto",
			HasInput:  true,
			ModelName: "MiniMax-M2.7",
			Workspace: "/Users/test/project",
			Width:     100,
		})
		plain := stripANSI(got)
		if !strings.Contains(plain, "auto mode") {
			t.Fatalf("status=%q, want mode label", plain)
		}
		if strings.Contains(plain, "MiniMax-M2.7") {
			t.Fatalf("mode should suppress model metadata: %q", plain)
		}
		if strings.Contains(plain, "project") {
			t.Fatalf("mode should suppress path metadata: %q", plain)
		}
	})

	t.Run("input_shows_model_path_and_tokens", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			HasInput:  true,
			ModelName: "openai/gpt-4.1",
			Workspace: "/Users/test/project",
			Width:     120,
			Metrics: StatusMetrics{
				PromptTokens:     1200,
				CompletionTokens: 752,
				ContextLimit:     1047576,
				OutputLimit:      16384,
				PromptEstimated:  true,
			},
		})
		plain := stripANSI(got)
		if !strings.Contains(plain, "gpt-4.1") || !strings.Contains(plain, "project") {
			t.Fatalf("status=%q, want model and workspace on left", plain)
		}
		if !strings.Contains(plain, "IN ~1.2k/1.0M 1%") {
			t.Fatalf("status=%q, want estimated input window", plain)
		}
		if !strings.Contains(plain, "OUT 752/16.4k 5%") {
			t.Fatalf("status=%q, want output window", plain)
		}
	})

	t.Run("idle_shows_shortcut_guide", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{Width: 80})
		plain := stripANSI(got)
		if !strings.Contains(plain, "? shortcuts") {
			t.Fatalf("status=%q, want shortcut guide", plain)
		}
	})

	t.Run("idle_shows_model_workspace_and_cost", func(t *testing.T) {
		got := RenderStatusV2(StatusV2Params{
			ModelName: "openai/gpt-4.1",
			Workspace: "/Users/test/project",
			Width:     120,
			Metrics: StatusMetrics{
				CostUSD: 0.123,
			},
		})
		plain := stripANSI(got)
		if !strings.Contains(plain, "gpt-4.1") || !strings.Contains(plain, "project") {
			t.Fatalf("status=%q, want model and workspace in idle state", plain)
		}
		if !strings.Contains(plain, "$0.123") {
			t.Fatalf("status=%q, want session cost in status row", plain)
		}
	})
}

func TestRenderFooterDock_InterruptAndSendHints(t *testing.T) {
	tests := []struct {
		name      string
		params    FooterDockParams
		want      string
		notWanted string
	}{
		{
			name: "idle_empty_hides_both",
			params: FooterDockParams{
				Width: 100,
			},
			want:      "shift+tab mode",
			notWanted: "enter to send",
		},
		{
			name: "idle_input_shows_enter",
			params: FooterDockParams{
				Width:    100,
				HasInput: true,
			},
			want:      "enter to send · shift+tab mode",
			notWanted: "esc to interrupt",
		},
		{
			name: "processing_interruptible_shows_esc",
			params: FooterDockParams{
				Width:        100,
				IsProcessing: true,
				CanInterrupt: true,
			},
			want:      "esc to interrupt · shift+tab mode",
			notWanted: "enter to send",
		},
		{
			name: "processing_non_interruptible_hides_esc",
			params: FooterDockParams{
				Width:        100,
				IsProcessing: true,
				CanInterrupt: false,
			},
			notWanted: "esc to interrupt",
		},
		{
			name: "processing_escape_owner_hides_esc",
			params: FooterDockParams{
				Width:          100,
				IsProcessing:   true,
				CanInterrupt:   true,
				HasEscapeOwner: true,
			},
			notWanted: "esc to interrupt",
		},
		{
			name: "blocking_overlay_hides_enter",
			params: FooterDockParams{
				Width:              100,
				HasInput:           true,
				HasBlockingOverlay: true,
			},
			notWanted: "enter to send",
		},
		{
			name: "narrow_footer_omits_shift_tab_hint",
			params: FooterDockParams{
				Width: 15,
			},
			notWanted: "shift+tab mode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(RenderFooterDock(tt.params))
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("dock=%q, want %q", got, tt.want)
			}
			if tt.notWanted != "" && strings.Contains(got, tt.notWanted) {
				t.Fatalf("dock=%q, should not contain %q", got, tt.notWanted)
			}
			if tt.name == "idle_empty_hides_both" && strings.Contains(got, "esc to interrupt") {
				t.Fatalf("dock=%q, should not contain esc hint", got)
			}
		})
	}
}

func TestRenderFooterDock_CtrlCPendingPreemptsNotice(t *testing.T) {
	got := stripANSI(RenderFooterDock(FooterDockParams{
		Width:        100,
		CtrlCPending: true,
		Notice: TransientNotice{
			Text: "Copied selection",
			Kind: NoticeSuccess,
		},
	}))
	if !strings.Contains(got, "Press Ctrl+C again to exit") {
		t.Fatalf("dock=%q, want ctrl+c confirmation", got)
	}
	if strings.Contains(got, "Copied selection") {
		t.Fatalf("dock=%q, notice should not hide ctrl+c confirmation", got)
	}
}
