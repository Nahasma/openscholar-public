package vim

// executeOperatorMotion executes an operator with a motion key and returns
// the resulting TextChange and the new VimMode.
//
// Supported operators:
//   - 'd' (delete): removes text described by the motion
//   - 'c' (change): removes text and enters Insert mode
//   - 'y' (yank):   copies text (no visible change in v1)
//
// Supported motions: h, j, k, l, w, b, e, 0, $
// Returns (nil, ModeOperatorPending) when the motion is not recognised,
// signalling that the caller should cancel the operator.
func executeOperatorMotion(op rune, motion rune, count int) (*TextChange, VimMode) {
	if count == 0 {
		count = 1
	}

	if !isMotionKey(motion) {
		// Unrecognised motion — cancel, stay in normal mode
		return nil, ModeNormal
	}

	var tc *TextChange

	switch op {
	case 'd':
		tc = buildDeleteChange(motion, count)
		return tc, ModeNormal

	case 'c':
		tc = buildDeleteChange(motion, count)
		return tc, ModeInsert

	case 'y':
		// Yank: no visible text change in v1
		return nil, ModeNormal

	default:
		return nil, ModeNormal
	}
}

// buildDeleteChange constructs a TextChange for a delete motion.
func buildDeleteChange(motion rune, count int) *TextChange {
	switch motion {
	case 'w', 'e':
		// Delete forward by approximate word width (count words).
		// The actual boundary is handled by the TUI layer; we signal
		// "delete count characters" as a proxy.
		return &TextChange{DeleteCount: count}
	case 'b':
		// Delete backward by word
		return &TextChange{DeleteCount: count}
	case 'h':
		return &TextChange{DeleteCount: count}
	case 'l':
		return &TextChange{DeleteCount: count}
	case '$':
		return &TextChange{DeleteToEOL: true}
	case '0':
		// Delete from start of line to cursor — represented as a large backward delete
		return &TextChange{DeleteCount: count}
	case 'j', 'k':
		// Delete lines
		return &TextChange{DeleteLine: true}
	default:
		return &TextChange{DeleteCount: count}
	}
}
