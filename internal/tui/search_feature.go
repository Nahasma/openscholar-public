package tui

import "github.com/Nahasma/openscholar-public/internal/tui/components"

// SearchFeature groups history search and text search state.
type SearchFeature struct {
	historySearch components.HistorySearchModel
	textSearch    components.TextSearchModel
}

// NewSearchFeature creates a SearchFeature with initialized models.
func NewSearchFeature() SearchFeature {
	return SearchFeature{
		historySearch: components.NewHistorySearch(),
		textSearch:    components.NewTextSearch(),
	}
}

// AnyVisible returns true if any search UI is visible.
func (s *SearchFeature) AnyVisible() bool {
	return s.historySearch.Visible() || s.textSearch.Visible()
}
