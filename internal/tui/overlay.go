package tui

import tea "github.com/charmbracelet/bubbletea"

// OverlayKind categorizes overlay behavior.
type OverlayKind int

const (
	// OverlayBlocking blocks all input to the layer beneath.
	OverlayBlocking OverlayKind = iota
	// OverlayNonBlocking allows chat interaction to coexist (e.g. help, copy-mode).
	OverlayNonBlocking
)

// OverlayResult is returned when an overlay completes.
type OverlayResult struct {
	Action string      // "accept" | "reject" | "dismiss"
	Data   any // optional payload
}

// Overlay is the common interface for all dialog overlays.
type Overlay interface {
	// ID returns a unique identifier for this overlay type (e.g. "permission").
	ID() string
	// Kind returns whether this overlay blocks input.
	Kind() OverlayKind
	// BlocksInput returns true if this overlay should consume all key events.
	BlocksInput() bool
	// Update processes a tea.Msg and returns the (possibly replaced) overlay,
	// an optional result (non-nil means the overlay is done), and a tea.Cmd.
	Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd)
	// View renders the overlay content for the given terminal dimensions.
	View(width, height int) string
}

// OverlayManager manages a stack of active overlays with a pending queue
// for blocking overlays.
type OverlayManager struct {
	stack   []Overlay
	pending []Overlay // queued blocking overlays (waiting for current blocking to finish)
}

// NewOverlayManager creates an empty OverlayManager.
func NewOverlayManager() *OverlayManager {
	return &OverlayManager{}
}

// Push adds an overlay to the stack.
// If it is blocking and there is already a blocking overlay active, it is queued.
func (om *OverlayManager) Push(o Overlay) {
	if o.Kind() == OverlayBlocking && om.HasBlocking() {
		om.pending = append(om.pending, o)
		return
	}
	om.stack = append(om.stack, o)
}

// Pop removes and returns the top overlay's result.
// If a pending blocking overlay exists, it is promoted to the stack.
func (om *OverlayManager) Pop() *OverlayResult {
	if len(om.stack) == 0 {
		return nil
	}
	top := om.stack[len(om.stack)-1]
	om.stack = om.stack[:len(om.stack)-1]

	// Promote next pending blocking overlay
	if top.Kind() == OverlayBlocking && len(om.pending) > 0 {
		next := om.pending[0]
		om.pending = om.pending[1:]
		om.stack = append(om.stack, next)
	}

	return &OverlayResult{Action: "dismiss"}
}

// PopByID removes the overlay with the given ID (if present) and returns its result.
func (om *OverlayManager) PopByID(id string) *OverlayResult {
	for i := len(om.stack) - 1; i >= 0; i-- {
		if om.stack[i].ID() == id {
			wasBlocking := om.stack[i].Kind() == OverlayBlocking
			om.stack = append(om.stack[:i], om.stack[i+1:]...)

			// Promote pending if a blocking overlay was removed
			if wasBlocking && len(om.pending) > 0 && !om.HasBlocking() {
				next := om.pending[0]
				om.pending = om.pending[1:]
				om.stack = append(om.stack, next)
			}
			return &OverlayResult{Action: "dismiss"}
		}
	}
	return nil
}

// Top returns the top-most overlay, or nil if the stack is empty.
func (om *OverlayManager) Top() Overlay {
	if len(om.stack) == 0 {
		return nil
	}
	return om.stack[len(om.stack)-1]
}

// IsEmpty returns true if there are no active overlays.
func (om *OverlayManager) IsEmpty() bool {
	return len(om.stack) == 0
}

// HasBlocking returns true if any active overlay is blocking.
func (om *OverlayManager) HasBlocking() bool {
	for _, o := range om.stack {
		if o.Kind() == OverlayBlocking {
			return true
		}
	}
	return false
}

// Depth returns the number of active overlays (not counting pending).
func (om *OverlayManager) Depth() int {
	return len(om.stack)
}

// PendingCount returns the number of queued blocking overlays.
func (om *OverlayManager) PendingCount() int {
	return len(om.pending)
}

// RouteKey sends a tea.KeyMsg to the top overlay.
// Returns (cmd, consumed). consumed=true means the overlay handled the key.
func (om *OverlayManager) RouteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if len(om.stack) == 0 {
		return nil, false
	}

	top := om.stack[len(om.stack)-1]
	if top.Kind() == OverlayNonBlocking {
		// Non-blocking overlays only consume Esc; all other keys pass through.
		if msg.Type != tea.KeyEsc {
			return nil, false
		}
		// Esc is routed to the overlay and always consumed.
		updated, result, cmd := top.Update(msg)
		om.stack[len(om.stack)-1] = updated
		if result != nil {
			om.stack = om.stack[:len(om.stack)-1]
			if top.Kind() == OverlayBlocking && len(om.pending) > 0 {
				next := om.pending[0]
				om.pending = om.pending[1:]
				om.stack = append(om.stack, next)
			}
		}
		return cmd, true
	}

	updated, result, cmd := top.Update(msg)
	om.stack[len(om.stack)-1] = updated

	if result != nil {
		// Overlay is done — pop it
		om.stack = om.stack[:len(om.stack)-1]

		// Promote pending blocking overlay
		if top.Kind() == OverlayBlocking && len(om.pending) > 0 {
			next := om.pending[0]
			om.pending = om.pending[1:]
			om.stack = append(om.stack, next)
		}
		return cmd, true
	}

	return cmd, true // blocking overlay always consumes
}

// RouteMsg sends a non-key tea.Msg to the top overlay (e.g. tick, window resize).
// Returns (cmd, consumed).
func (om *OverlayManager) RouteMsg(msg tea.Msg) (tea.Cmd, bool) {
	if len(om.stack) == 0 {
		return nil, false
	}

	top := om.stack[len(om.stack)-1]
	updated, result, cmd := top.Update(msg)
	om.stack[len(om.stack)-1] = updated

	if result != nil {
		om.stack = om.stack[:len(om.stack)-1]
		if top.Kind() == OverlayBlocking && len(om.pending) > 0 {
			next := om.pending[0]
			om.pending = om.pending[1:]
			om.stack = append(om.stack, next)
		}
		return cmd, true
	}

	return cmd, false
}

// FindByID returns the overlay with the given ID, or nil.
func (om *OverlayManager) FindByID(id string) Overlay {
	for _, o := range om.stack {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

// ReplaceTop replaces the top overlay in-place (used after Update returns a new Overlay).
func (om *OverlayManager) ReplaceTop(o Overlay) {
	if len(om.stack) > 0 {
		om.stack[len(om.stack)-1] = o
	}
}
