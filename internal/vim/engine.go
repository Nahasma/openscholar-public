package vim

// VimMode represents the current Vim editing mode.
type VimMode string

const (
	ModeNormal          VimMode = "NORMAL"
	ModeInsert          VimMode = "INSERT"
	ModeVisual          VimMode = "VISUAL"
	ModeVisualLine      VimMode = "VISUAL LINE"
	ModeVisualBlock     VimMode = "VISUAL BLOCK"
	ModeReplace         VimMode = "REPLACE"
	ModeReplaceOnce     VimMode = "REPLACE ONCE"
	ModeOperatorPending VimMode = "OPERATOR PENDING"
	ModeCommandLine     VimMode = "COMMAND"
	ModeSearch          VimMode = "SEARCH"
	ModeDisabled        VimMode = "DISABLED"
)

// String returns the display name for the mode.
func (m VimMode) String() string {
	if m == "" {
		return "UNKNOWN"
	}
	return string(m)
}

// TextChange describes a text modification.
type TextChange struct {
	DeleteCount int    // characters to delete before cursor
	InsertText  string // text to insert at cursor
	DeleteLine  bool   // delete entire current line
	DeleteToEOL bool   // delete from cursor to end of line
}

// CursorMove describes a cursor movement.
type CursorMove struct {
	DeltaX  int  // horizontal movement (positive = right)
	DeltaY  int  // vertical movement (positive = down)
	AbsX    int  // absolute X position (-1 = don't change)
	AbsY    int  // absolute Y position (-1 = don't change)
	ToEnd   bool // move to end of line
	ToStart bool // move to start of line
}

// Action records a repeatable action for dot-repeat.
type Action struct {
	Operator rune
	Motion   string
	Count    int
	Text     string // inserted text (for insert mode)
}

// Result is the outcome of processing a key press.
type Result struct {
	NewMode    VimMode
	TextChange *TextChange // nil = no text change
	CursorMove *CursorMove // nil = no cursor movement
	Command    string      // non-empty for : commands
	Handled    bool        // true if the key was consumed by vim
}

// Engine is the Vim state machine.
type Engine struct {
	mode          VimMode
	enabled       bool
	pendingOp     rune   // pending operator (d, c, y)
	count         int    // numeric prefix
	register      rune   // register (unused in v1)
	lastAction    *Action
	commandBuffer string
	searchBuffer  string
	insertBuffer  string // accumulates text during insert mode
}

// NewEngine creates a new Vim engine in disabled state.
func NewEngine() *Engine {
	return &Engine{
		mode:    ModeDisabled,
		enabled: false,
	}
}

// Mode returns the current vim mode.
func (e *Engine) Mode() VimMode {
	return e.mode
}

// Enabled returns whether vim mode is active.
func (e *Engine) Enabled() bool {
	return e.enabled
}

// SetEnabled enables or disables vim mode.
// When enabled, starts in Normal mode. When disabled, enters ModeDisabled.
func (e *Engine) SetEnabled(v bool) {
	e.enabled = v
	if v {
		e.mode = ModeNormal
		e.reset()
	} else {
		e.mode = ModeDisabled
	}
}

// Reset resets the engine to Normal mode, clearing all pending state.
func (e *Engine) Reset() {
	if e.enabled {
		e.mode = ModeNormal
	}
	e.reset()
}

// reset clears all transient state without changing the mode.
func (e *Engine) reset() {
	e.pendingOp = 0
	e.count = 0
	e.commandBuffer = ""
	e.searchBuffer = ""
	e.insertBuffer = ""
}

// effectiveCount returns count (defaulting to 1 if not set).
func (e *Engine) effectiveCount() int {
	if e.count == 0 {
		return 1
	}
	return e.count
}

// ProcessKey processes a key press and returns the result.
// In ModeDisabled, all keys pass through (Handled=false).
func (e *Engine) ProcessKey(key rune) Result {
	switch e.mode {
	case ModeDisabled:
		return Result{NewMode: ModeDisabled, Handled: false}
	case ModeNormal:
		return e.processNormal(key)
	case ModeInsert:
		return e.processInsert(key)
	case ModeVisual, ModeVisualLine:
		return e.processVisual(key)
	case ModeOperatorPending:
		return e.processOperatorPending(key)
	case ModeCommandLine:
		return e.processCommandLine(key)
	case ModeSearch:
		return e.processSearch(key)
	case ModeReplace:
		return e.processReplace(key)
	default:
		return Result{NewMode: e.mode, Handled: false}
	}
}

// processNormal handles keys in Normal mode.
func (e *Engine) processNormal(key rune) Result {
	// Numeric prefix: 1-9 always start a count; 0 is only a count digit if count > 0
	if key >= '1' && key <= '9' {
		e.count = e.count*10 + int(key-'0')
		return Result{NewMode: ModeNormal, Handled: true}
	}
	if key == '0' && e.count > 0 {
		e.count = e.count * 10
		return Result{NewMode: ModeNormal, Handled: true}
	}

	cnt := e.effectiveCount()

	switch key {
	// Mode transitions
	case 'i':
		e.reset()
		e.mode = ModeInsert
		return Result{NewMode: ModeInsert, Handled: true}

	case 'a':
		e.reset()
		e.mode = ModeInsert
		return Result{
			NewMode:    ModeInsert,
			CursorMove: &CursorMove{DeltaX: 1},
			Handled:    true,
		}

	case 'o':
		e.reset()
		e.mode = ModeInsert
		return Result{
			NewMode:    ModeInsert,
			TextChange: &TextChange{InsertText: "\n"},
			CursorMove: &CursorMove{DeltaY: 1},
			Handled:    true,
		}

	case 'O':
		e.reset()
		e.mode = ModeInsert
		return Result{
			NewMode:    ModeInsert,
			TextChange: &TextChange{InsertText: "\n"},
			CursorMove: &CursorMove{DeltaY: -1},
			Handled:    true,
		}

	case 'v':
		e.reset()
		e.mode = ModeVisual
		return Result{NewMode: ModeVisual, Handled: true}

	case 'V':
		e.reset()
		e.mode = ModeVisualLine
		return Result{NewMode: ModeVisualLine, Handled: true}

	case 'R':
		e.reset()
		e.mode = ModeReplace
		return Result{NewMode: ModeReplace, Handled: true}

	// Cursor motions
	case 'h', 'j', 'k', 'l', 'w', 'b', 'e':
		mv := executeMotion(key, cnt)
		e.reset()
		return Result{NewMode: ModeNormal, CursorMove: mv, Handled: true}

	case '0':
		// count is 0 here (checked above), so this is start-of-line
		e.reset()
		return Result{
			NewMode:    ModeNormal,
			CursorMove: &CursorMove{ToStart: true, AbsX: -1, AbsY: -1},
			Handled:    true,
		}

	case '$':
		e.reset()
		return Result{
			NewMode:    ModeNormal,
			CursorMove: &CursorMove{ToEnd: true, AbsX: -1, AbsY: -1},
			Handled:    true,
		}

	// Editing commands
	case 'x':
		e.reset()
		tc := &TextChange{DeleteCount: cnt}
		act := &Action{Operator: 'x', Count: cnt}
		e.lastAction = act
		return Result{NewMode: ModeNormal, TextChange: tc, Handled: true}

	case 'd':
		e.mode = ModeOperatorPending
		e.pendingOp = 'd'
		return Result{NewMode: ModeOperatorPending, Handled: true}

	case 'c':
		e.mode = ModeOperatorPending
		e.pendingOp = 'c'
		return Result{NewMode: ModeOperatorPending, Handled: true}

	case 'y':
		e.mode = ModeOperatorPending
		e.pendingOp = 'y'
		return Result{NewMode: ModeOperatorPending, Handled: true}

	case 'D':
		e.reset()
		tc := &TextChange{DeleteToEOL: true}
		e.lastAction = &Action{Operator: 'D', Count: 1}
		return Result{NewMode: ModeNormal, TextChange: tc, Handled: true}

	case 'p':
		// paste placeholder
		e.reset()
		return Result{NewMode: ModeNormal, Handled: true}

	case '.':
		res := e.DotRepeat()
		return res

	case ':':
		e.reset()
		e.mode = ModeCommandLine
		e.commandBuffer = ""
		return Result{NewMode: ModeCommandLine, Handled: true}

	case '/':
		e.reset()
		e.mode = ModeSearch
		e.searchBuffer = ""
		return Result{NewMode: ModeSearch, Handled: true}

	case 0x1b: // Esc
		e.reset()
		return Result{NewMode: ModeNormal, Handled: true}

	default:
		return Result{NewMode: ModeNormal, Handled: false}
	}
}

// processInsert handles keys in Insert mode.
func (e *Engine) processInsert(key rune) Result {
	switch key {
	case 0x1b: // Esc → back to Normal
		// Save inserted text to lastAction
		if e.insertBuffer != "" {
			if e.lastAction == nil {
				e.lastAction = &Action{}
			}
			e.lastAction.Text = e.insertBuffer
		}
		e.insertBuffer = ""
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}
	default:
		// Accumulate into insertBuffer but pass key through to textarea
		e.insertBuffer += string(key)
		return Result{NewMode: ModeInsert, Handled: false}
	}
}

// processVisual handles keys in Visual / VisualLine mode.
func (e *Engine) processVisual(key rune) Result {
	currentMode := e.mode
	switch key {
	case 0x1b: // Esc
		e.reset()
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}
	case 'd', 'x':
		e.reset()
		e.mode = ModeNormal
		return Result{
			NewMode:    ModeNormal,
			TextChange: &TextChange{DeleteCount: 1},
			Handled:    true,
		}
	case 'y':
		e.reset()
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}
	default:
		return Result{NewMode: currentMode, Handled: false}
	}
}

// processOperatorPending handles keys in OperatorPending mode.
func (e *Engine) processOperatorPending(key rune) Result {
	cnt := e.effectiveCount()
	op := e.pendingOp

	switch key {
	case 0x1b: // Esc → cancel
		e.reset()
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}

	case 'd', 'c', 'y':
		// Doubled operator: dd, cc, yy
		if key == op {
			e.reset()
			switch op {
			case 'd':
				e.lastAction = &Action{Operator: 'd', Motion: "d", Count: cnt}
				e.mode = ModeNormal
				return Result{
					NewMode:    ModeNormal,
					TextChange: &TextChange{DeleteLine: true},
					Handled:    true,
				}
			case 'c':
				e.lastAction = &Action{Operator: 'c', Motion: "c", Count: cnt}
				e.mode = ModeInsert
				return Result{
					NewMode:    ModeInsert,
					TextChange: &TextChange{DeleteLine: true},
					Handled:    true,
				}
			case 'y':
				// yank line — no visible change
				e.lastAction = &Action{Operator: 'y', Motion: "y", Count: cnt}
				e.mode = ModeNormal
				return Result{NewMode: ModeNormal, Handled: true}
			}
		}
		// Different operator key in operator pending — treat as cancel + re-process
		e.reset()
		e.mode = ModeNormal
		return e.processNormal(key)

	default:
		// Motion key
		tc, newMode := executeOperatorMotion(op, key, cnt)
		if tc != nil || newMode != ModeOperatorPending {
			e.lastAction = &Action{Operator: op, Motion: string(key), Count: cnt}
			e.reset()
			e.mode = newMode
			return Result{
				NewMode:    newMode,
				TextChange: tc,
				CursorMove: nil,
				Handled:    true,
			}
		}
		// Unknown motion — cancel
		e.reset()
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}
	}
}

// processCommandLine handles keys in CommandLine mode.
func (e *Engine) processCommandLine(key rune) Result {
	switch key {
	case 0x1b: // Esc → cancel
		e.commandBuffer = ""
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}

	case '\r', '\n': // Enter → execute command
		cmd := e.commandBuffer
		e.commandBuffer = ""
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Command: cmd, Handled: true}

	case 0x7f, '\b': // Backspace
		if len(e.commandBuffer) > 0 {
			e.commandBuffer = e.commandBuffer[:len(e.commandBuffer)-1]
		}
		return Result{NewMode: ModeCommandLine, Handled: true}

	default:
		e.commandBuffer += string(key)
		return Result{NewMode: ModeCommandLine, Handled: true}
	}
}

// processSearch handles keys in Search mode.
func (e *Engine) processSearch(key rune) Result {
	switch key {
	case 0x1b: // Esc → cancel
		e.searchBuffer = ""
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}

	case '\r', '\n': // Enter → execute search
		_ = e.searchBuffer
		e.searchBuffer = ""
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}

	case 0x7f, '\b': // Backspace
		if len(e.searchBuffer) > 0 {
			e.searchBuffer = e.searchBuffer[:len(e.searchBuffer)-1]
		}
		return Result{NewMode: ModeSearch, Handled: true}

	default:
		e.searchBuffer += string(key)
		return Result{NewMode: ModeSearch, Handled: true}
	}
}

// processReplace handles keys in Replace mode.
func (e *Engine) processReplace(key rune) Result {
	switch key {
	case 0x1b: // Esc → back to Normal
		e.mode = ModeNormal
		return Result{NewMode: ModeNormal, Handled: true}
	default:
		// Replace character under cursor
		tc := &TextChange{DeleteCount: 1, InsertText: string(key)}
		return Result{NewMode: ModeReplace, TextChange: tc, Handled: true}
	}
}

// DotRepeat replays the last repeatable action.
func (e *Engine) DotRepeat() Result {
	if e.lastAction == nil {
		return Result{NewMode: e.mode, Handled: false}
	}
	act := e.lastAction
	cnt := act.Count
	if cnt == 0 {
		cnt = 1
	}

	switch act.Operator {
	case 'x':
		return Result{
			NewMode:    ModeNormal,
			TextChange: &TextChange{DeleteCount: cnt},
			Handled:    true,
		}
	case 'd':
		tc, newMode := executeOperatorMotion('d', []rune(act.Motion)[0], cnt)
		if act.Motion == "d" {
			return Result{
				NewMode:    ModeNormal,
				TextChange: &TextChange{DeleteLine: true},
				Handled:    true,
			}
		}
		_ = newMode
		return Result{NewMode: ModeNormal, TextChange: tc, Handled: true}
	case 'c':
		if act.Motion == "c" {
			e.mode = ModeInsert
			return Result{
				NewMode:    ModeInsert,
				TextChange: &TextChange{DeleteLine: true},
				Handled:    true,
			}
		}
		tc, newMode := executeOperatorMotion('c', []rune(act.Motion)[0], cnt)
		e.mode = newMode
		return Result{NewMode: newMode, TextChange: tc, Handled: true}
	case 'D':
		return Result{
			NewMode:    ModeNormal,
			TextChange: &TextChange{DeleteToEOL: true},
			Handled:    true,
		}
	default:
		return Result{NewMode: e.mode, Handled: false}
	}
}
