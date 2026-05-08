package components

// Unified Unicode figures for TUI rendering.
// All symbols are monospace-safe Unicode — no emoji.
const (
	FigSpinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" // braille spinner
	FigBullet        = "∙"                   // separator
	FigThinking      = "∴"                   // thinking prefix (CC: THEREFORE sign)
	FigConnector     = "⎿"                   // CC MessageResponse connector
	FigDiamondOpen   = "◇"                   // running/pending
	FigDiamondFill   = "◆"                   // completed/failed
	FigCircleFill    = "●"                   // active status dot
	FigCircleOpen    = "○"                   // inactive
	FigTreeMid       = "├─"                  // tree mid
	FigTreeEnd       = "└─"                  // tree end
	FigTreeVert      = "│"                   // tree vertical
	FigPrompt        = "❯"                   // input prompt
	FigExpand        = "▶"                   // collapsed marker
	FigCollapse      = "▼"                   // expanded marker
	FigCheckmark     = "✓"                   // success
	FigCross         = "✗"                   // failure
	FigHeavyLine     = "━"                   // heavy separator
	FigBlockquote    = "▎"                   // blockquote
	FigCursor        = "▌"                   // streaming cursor
	FigAttachment    = "+"                   // attachment indicator
	FigBlackCircle   = "⏺"                   // CC message dot prefix (U+23FA)
	FigFleuron       = "✻"                   // CC compact boundary marker (U+273B)
)
