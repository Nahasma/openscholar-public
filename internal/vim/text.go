package vim

// TextObject represents a vim text object (iw, aw, i", a", etc.)
type TextObject struct {
	Inner bool // inner (i) vs around (a)
	Kind  rune // w, ", (, [, {, etc.
}

// parseTextObject parses a text object from two characters.
// The first character must be 'i' (inner) or 'a' (around).
// Returns the TextObject and true on success, or zero value and false if
// the pair is not a supported text object.
//
// v1 supports: iw, aw only.
func parseTextObject(first, second rune) (TextObject, bool) {
	var inner bool
	switch first {
	case 'i':
		inner = true
	case 'a':
		inner = false
	default:
		return TextObject{}, false
	}

	switch second {
	case 'w': // word
		return TextObject{Inner: inner, Kind: 'w'}, true
	// Future text objects (v2+):
	// case '"', '\'', '`', '(', ')', '[', ']', '{', '}', '<', '>':
	//     return TextObject{Inner: inner, Kind: second}, true
	default:
		return TextObject{}, false
	}
}

// applyTextObject returns a TextChange that represents operating on the
// given text object with the given operator. This is a v1 stub — actual
// boundary detection is delegated to the TUI layer.
func applyTextObject(op rune, obj TextObject) (*TextChange, VimMode) {
	switch op {
	case 'd':
		return &TextChange{DeleteCount: 1}, ModeNormal
	case 'c':
		return &TextChange{DeleteCount: 1}, ModeInsert
	case 'y':
		return nil, ModeNormal
	default:
		return nil, ModeNormal
	}
}
