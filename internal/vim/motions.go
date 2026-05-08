package vim

// executeMotion returns a CursorMove for the given motion key and count.
// Supported motions: h (left), j (down), k (up), l (right), w (word forward),
// b (word backward), e (word end), 0 (start of line), $ (end of line).
func executeMotion(key rune, count int) *CursorMove {
	if count == 0 {
		count = 1
	}
	switch key {
	case 'h':
		return &CursorMove{DeltaX: -count, AbsX: -1, AbsY: -1}
	case 'l':
		return &CursorMove{DeltaX: count, AbsX: -1, AbsY: -1}
	case 'j':
		return &CursorMove{DeltaY: count, AbsX: -1, AbsY: -1}
	case 'k':
		return &CursorMove{DeltaY: -count, AbsX: -1, AbsY: -1}
	case 'w':
		// Word forward: treated as moving right by a word-width approximation.
		// In v1, we represent word motion as a relative move; the TUI layer
		// is responsible for finding the actual word boundary.
		return &CursorMove{DeltaX: count, AbsX: -1, AbsY: -1}
	case 'b':
		// Word backward
		return &CursorMove{DeltaX: -count, AbsX: -1, AbsY: -1}
	case 'e':
		// End of word forward
		return &CursorMove{DeltaX: count, AbsX: -1, AbsY: -1}
	case '0':
		return &CursorMove{ToStart: true, AbsX: -1, AbsY: -1}
	case '$':
		return &CursorMove{ToEnd: true, AbsX: -1, AbsY: -1}
	default:
		return nil
	}
}

// isMotionKey reports whether the rune is a recognised motion key.
func isMotionKey(key rune) bool {
	switch key {
	case 'h', 'j', 'k', 'l', 'w', 'b', 'e', '0', '$':
		return true
	}
	return false
}
