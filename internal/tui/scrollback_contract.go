package tui

// Main-screen resize contract:
//
// OpenScholar uses the terminal main screen by default.
//
// Main-screen frame bytes are staged and rendered through a dedicated output
// controller. The standard Bubble Tea frame placeholder is replaced by the
// controller output; non-frame terminal bytes pass through unchanged.
//
// Main-screen scrollback is terminal-native in non-fullscreen mode. OpenScholar
// does not commit prior transcript lines through renderer-managed commit queues.
// As the frame grows, terminal rows are created via CR/LF and terminal
// scrollback naturally accumulates historical output.
//
// Reset contract:
// - Default (`OS_TUI_MAIN_RESET` unset/empty): `ESC[2J` + `ESC[3J` + `ESC[H]`.
// - Emergency override (`OS_TUI_MAIN_RESET=visible`): `ESC[2J` + `ESC[H]`.
//
// Startup and ordinary ticks do not reset the terminal. Renderer-decided
// resets (resize/offscreen/return-main) use the selected reset mode.
//
// Reset/return-main transactions are ordered as:
// clear sequence + current natural-height frame.
//
// Fullscreen/alt-screen contract:
//
// Fullscreen keeps transcript history application-owned through the message
// source, BlockList, height cache, and virtual transcript path. That path is
// the supported way to render full history at the current terminal width. It
// must not flush transcript content to native scrollback.
