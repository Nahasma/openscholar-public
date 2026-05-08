package tui

import "github.com/openscholar/openscholar/internal/debug"

func anchorSummary(anchor *messageSliceAnchor) string {
	if anchor == nil {
		return "-"
	}
	return anchor.MessageID
}

func streamFlushSummary(state *streamLineFlushState) string {
	if state == nil {
		return "-"
	}
	return state.MessageID
}

func (m Model) frameMeta(reason string) debug.TUIFrameMeta {
	frame := m.chat.mainScreenFrame
	pending := false
	applied := false
	if frame != nil {
		pending = frame.PendingReset
		applied = frame.AppliedReset
	}
	mainSlice := m.mainScreenFrameSliceAnchor()
	resetReason := ""
	prevLines := 0
	frameLines := m.mainScreenTranscriptLineCount()
	if frame != nil {
		resetReason = frame.Reason
		prevLines = frame.PrevLines
		if frame.SliceAnchor != "" {
			mainSlice = frame.SliceAnchor
		}
	}
	mainStats := m.mainOutput.Stats()
	if mainStats.ResetReason != "" {
		resetReason = mainStats.ResetReason
	}
	if mainStats.PrevLines > 0 {
		prevLines = mainStats.PrevLines
	}
	return debug.TUIFrameMeta{
		Reason:                    reason,
		State:                     m.stateName(),
		Width:                     m.width,
		Height:                    m.height,
		Fullscreen:                m.isFullscreenMode(),
		ResizeEpoch:               m.resizeEpoch,
		MainScreenViewportOwned:   m.chat.mainScreenViewportOwned,
		MainScreenResetPending:    pending,
		MainScreenResetApplied:    applied,
		FlushedAnchor:             anchorSummary(m.chat.flushedAnchor),
		LiveTailAnchor:            anchorSummary(m.chat.liveTailAnchor),
		OwnedStartAnchor:          anchorSummary(m.chat.mainScreenOwnedStart),
		StreamFlush:               streamFlushSummary(m.chat.activeStreamFlush),
		OwnedStreamFlush:          streamFlushSummary(m.chat.mainScreenOwnedStream),
		MainScreenResetReason:     resetReason,
		MainScreenSliceAnchor:     mainSlice,
		MainScreenPrevLines:       prevLines,
		MainScreenNextLines:       mainStats.NextLines,
		MainScreenFrameLines:      frameLines,
		MainScreenViewportHeight:  mainStats.ViewportHeight,
		MainScreenRendererEnabled: m.mainOutput != nil,
		MainScreenResetMode:       string(m.mainResetMode),
		MainScreenFrameSeq:        mainStats.FrameSeq,
		MainScreenFullReset:       mainStats.FullReset,
		MainScreenOffscreenReset:  mainStats.OffscreenReset,
	}
}

func (m Model) stateName() string {
	switch m.state {
	case stateChat:
		return "chat"
	case statePermission:
		return "permission"
	case stateHelp:
		return "help"
	case stateSessionBrowser:
		return "session_browser"
	case statePlanApproval:
		return "plan_approval"
	case stateClarification:
		return "clarification"
	case stateModelSelection:
		return "model_selection"
	case stateInitRequired:
		return "init_required"
	case stateInitWizard:
		return "init_wizard"
	case stateConfigWizard:
		return "config_wizard"
	case stateCheckpoint:
		return "checkpoint"
	case stateTemplateSelection:
		return "template_selection"
	case stateWorkspaceSelection:
		return "workspace_selection"
	case stateResearchSuggestion:
		return "research_suggestion"
	case stateCopyMode:
		return "copy_mode"
	default:
		return "unknown"
	}
}

func (m Model) logFrame(reason, rendered string) {
	if m.debugTrace == nil {
		return
	}
	m.debugTrace.LogFrame(m.frameMeta(reason), rendered)
}

func (m Model) logScrollback(reason, rendered string) {
	if m.debugTrace == nil {
		return
	}
	m.debugTrace.LogScrollback(m.frameMeta(reason), rendered)
}
