package picker

// State is the complete picker state, UI-agnostic.
// Can be used by TUI, GUI, or any other frontend.
type State struct {
	Active   bool
	Query    string      // search text after @
	Items    []FileEntry // current matched results
	Selected int         // selected index
	PageSize int         // max visible items (default 8)
}

// New creates a picker state with the given page size.
func New(pageSize int) *State {
	if pageSize <= 0 {
		pageSize = 8
	}
	return &State{PageSize: pageSize}
}

// UpdateQuery updates the search query and re-matches against the index.
func (s *State) UpdateQuery(query string, index []FileEntry) {
	prev := FileEntry{}
	hasPrev := s.Selected >= 0 && s.Selected < len(s.Items)
	if hasPrev {
		prev = s.Items[s.Selected]
	}
	s.Query = query
	s.Items = FuzzyMatch(index, query, s.PageSize*3) // fetch extra for scrolling
	s.Active = len(s.Items) > 0
	s.Selected = 0
	if hasPrev {
		for i, entry := range s.Items {
			if entry.RelPath == prev.RelPath && entry.IsDir == prev.IsDir {
				s.Selected = i
				break
			}
		}
	}
}

// MoveUp moves selection up.
func (s *State) MoveUp() {
	if len(s.Items) == 0 {
		return
	}
	if s.Selected <= 0 {
		s.Selected = len(s.Items) - 1
		return
	}
	s.Selected--
}

// MoveDown moves selection down.
func (s *State) MoveDown() {
	if len(s.Items) == 0 {
		return
	}
	if s.Selected >= len(s.Items)-1 {
		s.Selected = 0
		return
	}
	s.Selected++
}

// Select returns the currently selected entry and whether it's a directory.
func (s *State) Select() (FileEntry, bool) {
	if s.Selected >= 0 && s.Selected < len(s.Items) {
		entry := s.Items[s.Selected]
		return entry, entry.IsDir
	}
	return FileEntry{}, false
}

// Reset deactivates the picker.
func (s *State) Reset() {
	s.Active = false
	s.Query = ""
	s.Items = nil
	s.Selected = 0
}

// VisibleItems returns the items currently visible (paged around selected).
func (s *State) VisibleItems() []FileEntry {
	if len(s.Items) <= s.PageSize {
		return s.Items
	}

	// Window around selected item
	start := s.Selected - s.PageSize/2
	if start < 0 {
		start = 0
	}
	end := start + s.PageSize
	if end > len(s.Items) {
		end = len(s.Items)
		start = end - s.PageSize
		if start < 0 {
			start = 0
		}
	}
	return s.Items[start:end]
}

// VisibleSelectedIndex returns the selected index within the visible window.
func (s *State) VisibleSelectedIndex() int {
	if len(s.Items) <= s.PageSize {
		return s.Selected
	}
	start := s.Selected - s.PageSize/2
	if start < 0 {
		start = 0
	}
	if start+s.PageSize > len(s.Items) {
		start = len(s.Items) - s.PageSize
		if start < 0 {
			start = 0
		}
	}
	return s.Selected - start
}
