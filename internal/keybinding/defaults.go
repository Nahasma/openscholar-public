package keybinding

// DefaultRegistry creates a registry with all default key bindings.
// These match the existing hard-coded shortcuts in tui/key_handlers.go.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Global scope: available in all states
	r.Register(Binding{
		ID: "ctrl_c", Scope: ScopeGlobal, Key: "ctrl+c",
		Description: "Cancel processing or quit (double press)", Reserved: true,
	}, ActionCancelOrQuit)

	r.Register(Binding{
		ID: "ctrl_l", Scope: ScopeGlobal, Key: "ctrl+l",
		Description: "Redraw screen", Reserved: false,
	}, ActionRedraw)

	// Chat scope: available only in chat state
	r.Register(Binding{
		ID: "shift_tab", Scope: ScopeChat, Key: "shift+tab",
		Description: "Cycle permission mode", Reserved: false,
	}, ActionCycleMode)

	r.Register(Binding{
		ID: "ctrl_n", Scope: ScopeChat, Key: "ctrl+n",
		Description: "New session", Reserved: false,
	}, ActionNewSession)

	r.Register(Binding{
		ID: "ctrl_p", Scope: ScopeChat, Key: "ctrl+p",
		Description: "Open session browser", Reserved: false,
	}, ActionSessionBrowser)

	r.Register(Binding{
		ID: "ctrl_r", Scope: ScopeChat, Key: "ctrl+r",
		Description: "Search command history", Reserved: false,
	}, ActionHistorySearch)

	r.Register(Binding{
		ID: "ctrl_f", Scope: ScopeChat, Key: "ctrl+f",
		Description: "Search chat text", Reserved: false,
	}, ActionTextSearch)

	r.Register(Binding{
		ID: "ctrl_j", Scope: ScopeChat, Key: "ctrl+j",
		Description: "Navigate to next tool call", Reserved: false,
	}, ActionToolNavDown)

	r.Register(Binding{
		ID: "ctrl_k", Scope: ScopeChat, Key: "ctrl+k",
		Description: "Navigate to previous tool call", Reserved: false,
	}, ActionToolNavUp)

	r.Register(Binding{
		ID: "ctrl_o", Scope: ScopeChat, Key: "ctrl+o",
		Description: "Toggle expand/collapse tool call", Reserved: false,
	}, ActionToolToggle)

	r.Register(Binding{
		ID: "escape", Scope: ScopeChat, Key: "esc",
		Description: "Cancel current operation", Reserved: true,
	}, ActionCancel)

	r.Register(Binding{
		ID: "enter", Scope: ScopeChat, Key: "enter",
		Description: "Send message", Reserved: true,
	}, ActionSend)

	r.Register(Binding{
		ID: "question_mark", Scope: ScopeChat, Key: "?",
		Description: "Show help (when input empty)", Reserved: false,
	}, ActionHelp)

	r.Register(Binding{
		ID: "page_up", Scope: ScopeChat, Key: "pgup",
		Description: "Scroll viewport up", Reserved: false,
	}, ActionPageUp)

	r.Register(Binding{
		ID: "page_down", Scope: ScopeChat, Key: "pgdown",
		Description: "Scroll viewport down", Reserved: false,
	}, ActionPageDown)

	// Overlay scope: active when overlay is visible
	r.Register(Binding{
		ID: "enter_overlay", Scope: ScopeOverlay, Key: "enter",
		Description: "Accept overlay option", Reserved: true,
	}, ActionOverlayAccept)

	r.Register(Binding{
		ID: "esc_overlay", Scope: ScopeOverlay, Key: "esc",
		Description: "Dismiss overlay", Reserved: true,
	}, ActionOverlayDismiss)

	r.Register(Binding{
		ID: "tab_overlay", Scope: ScopeOverlay, Key: "tab",
		Description: "Toggle overlay input mode", Reserved: false,
	}, ActionOverlayInputToggle)

	// Chat scope: new scroll + copy actions
	r.Register(Binding{
		ID: "scroll_top", Scope: ScopeChat, Key: "home",
		Description: "Scroll to top", Reserved: false,
	}, ActionScrollTop)

	r.Register(Binding{
		ID: "scroll_bottom", Scope: ScopeChat, Key: "end",
		Description: "Scroll to bottom", Reserved: false,
	}, ActionScrollBottom)

	r.Register(Binding{
		ID: "copy_response", Scope: ScopeChat, Key: "ctrl+y",
		Description: "Copy last assistant response", Reserved: false,
	}, ActionCopyResponse)

	r.Register(Binding{
		ID: "copy_code_block", Scope: ScopeChat, Key: "ctrl+shift+y",
		Description: "Copy focused code block", Reserved: false,
	}, ActionCopyCodeBlock)

	// Vim scope: active when vim normal mode is enabled.
	// Note: compound keys (gg, yy) are handled by VimMode.HandleKey pending logic,
	// not registered here as single-key bindings.
	r.Register(Binding{
		ID: "vim_j", Scope: ScopeVim, Key: "j",
		Description: "Scroll viewport down (vim)", Reserved: false,
	}, ActionVimScrollDown)

	r.Register(Binding{
		ID: "vim_k", Scope: ScopeVim, Key: "k",
		Description: "Scroll viewport up (vim)", Reserved: false,
	}, ActionVimScrollUp)

	// Note: gg (goto top) and yy (yank block) are compound keys handled by
	// VimMode.HandleKey pending logic, not registered here as single-key bindings.

	r.Register(Binding{
		ID: "vim_G", Scope: ScopeVim, Key: "G",
		Description: "Goto bottom (vim)", Reserved: false,
	}, ActionVimGotoBottom)

	r.Register(Binding{
		ID: "vim_slash", Scope: ScopeVim, Key: "/",
		Description: "Search (vim)", Reserved: false,
	}, ActionVimSearch)

	r.Register(Binding{
		ID: "vim_n", Scope: ScopeVim, Key: "n",
		Description: "Next search match (vim)", Reserved: false,
	}, ActionVimNextMatch)

	r.Register(Binding{
		ID: "vim_N", Scope: ScopeVim, Key: "N",
		Description: "Previous search match (vim)", Reserved: false,
	}, ActionVimPrevMatch)

	r.Register(Binding{
		ID: "vim_o", Scope: ScopeVim, Key: "o",
		Description: "Toggle fold (vim)", Reserved: false,
	}, ActionVimToggleFold)

	r.Register(Binding{
		ID: "vim_i", Scope: ScopeVim, Key: "i",
		Description: "Enter insert mode (vim)", Reserved: false,
	}, ActionVimInsertMode)

	r.Register(Binding{
		ID: "vim_ctrl_d", Scope: ScopeVim, Key: "ctrl+d",
		Description: "Half page down (vim)", Reserved: false,
	}, ActionVimHalfPageDown)

	r.Register(Binding{
		ID: "vim_ctrl_u", Scope: ScopeVim, Key: "ctrl+u",
		Description: "Half page up (vim)", Reserved: false,
	}, ActionVimHalfPageUp)

	return r
}
