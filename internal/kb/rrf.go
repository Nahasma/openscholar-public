package kb

import "sort"

// RankedItem represents a scored item in a ranked list for RRF merging.
type RankedItem struct {
	PaperID string
	NodeID  string
	Title   string
	Snippet string
	Score   float64
}

// RRFMerge combines multiple ranked lists using Reciprocal Rank Fusion.
// Each input list is already sorted by relevance (best first).
// k is the smoothing constant (default 60).
func RRFMerge(k int, lists ...[]RankedItem) []RankedItem {
	if k <= 0 {
		k = 60
	}

	scores := make(map[string]*RankedItem) // key: "paperID:nodeID"

	for _, list := range lists {
		for rank, item := range list {
			key := item.PaperID + ":" + item.NodeID
			if existing, ok := scores[key]; ok {
				existing.Score += 1.0 / float64(k+rank+1)
			} else {
				copied := item
				copied.Score = 1.0 / float64(k+rank+1)
				scores[key] = &copied
			}
		}
	}

	result := make([]RankedItem, 0, len(scores))
	for _, item := range scores {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result
}

// ftsToRanked converts FTSResult list to RankedItem list (preserving order).
func ftsToRanked(results []FTSResult) []RankedItem {
	items := make([]RankedItem, len(results))
	for i, r := range results {
		items[i] = RankedItem{
			PaperID: r.PaperID,
			NodeID:  r.NodeID,
			Title:   r.Title,
			Snippet: r.Snippet,
		}
	}
	return items
}
