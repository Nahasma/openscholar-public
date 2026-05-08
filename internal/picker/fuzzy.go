package picker

import (
	"sort"
	"strings"
)

// FuzzyMatch returns entries matching query, sorted by relevance (best first).
func FuzzyMatch(entries []FileEntry, query string, limit int) []FileEntry {
	if query == "" {
		// No query: return top entries sorted by depth (shallow first), dirs first
		result := make([]FileEntry, len(entries))
		copy(result, entries)
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].Depth != result[j].Depth {
				return result[i].Depth < result[j].Depth
			}
			if result[i].IsDir != result[j].IsDir {
				return result[i].IsDir
			}
			return result[i].RelPath < result[j].RelPath
		})
		if len(result) > limit {
			result = result[:limit]
		}
		return result
	}

	// If query contains /, split into dir prefix + file query
	dirPrefix := ""
	fileQuery := query
	if idx := strings.LastIndex(query, "/"); idx >= 0 {
		dirPrefix = query[:idx+1]
		fileQuery = query[idx+1:]
	}

	type scored struct {
		entry FileEntry
		score int
	}

	var matches []scored
	queryLower := strings.ToLower(fileQuery)

	for _, e := range entries {
		// If dir prefix specified, filter to that subtree
		if dirPrefix != "" && !strings.HasPrefix(strings.ToLower(e.RelPath), strings.ToLower(dirPrefix)) {
			continue
		}

		// Match against the path portion after the prefix
		matchPath := e.RelPath
		if dirPrefix != "" {
			matchPath = e.RelPath[len(dirPrefix):]
		}

		if fileQuery == "" {
			// Empty file query after prefix: show all in that directory
			// Only show direct children (no deeper nesting)
			if !strings.Contains(matchPath, "/") || (e.IsDir && strings.Count(matchPath, "/") == 0) {
				matches = append(matches, scored{entry: e, score: e.Depth * 10})
			}
			continue
		}

		matched, score := fuzzyScore(matchPath, queryLower)
		if matched {
			matches = append(matches, scored{entry: e, score: score})
		}
	}

	// Sort by score (lower = better), then dirs first, then alphabetical
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		if matches[i].entry.IsDir != matches[j].entry.IsDir {
			return matches[i].entry.IsDir
		}
		return matches[i].entry.RelPath < matches[j].entry.RelPath
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	result := make([]FileEntry, len(matches))
	for i, m := range matches {
		result[i] = m.entry
	}
	return result
}

// fuzzyScore computes a match score for subsequence matching.
// Lower score = better match. Returns (false, 0) if no match.
func fuzzyScore(path, query string) (bool, int) {
	if len(query) == 0 {
		return true, 0
	}

	pathLower := strings.ToLower(path)
	queryRunes := []rune(query)
	pathRunes := []rune(pathLower)

	// Check if query is a subsequence of path
	qi := 0
	score := 0
	lastMatch := -1
	consecutiveBonus := 0

	for pi := 0; pi < len(pathRunes) && qi < len(queryRunes); pi++ {
		if pathRunes[pi] == queryRunes[qi] {
			// Consecutive match bonus
			if lastMatch == pi-1 {
				consecutiveBonus++
			}
			// Gap penalty
			if lastMatch >= 0 {
				gap := pi - lastMatch - 1
				score += gap * 2
			} else {
				// Penalty for late first match
				score += pi
			}
			lastMatch = pi
			qi++
		}
	}

	if qi < len(queryRunes) {
		return false, 0 // not all query chars matched
	}

	// Apply bonuses
	score -= consecutiveBonus * 3 // reward consecutive matches

	// Prefix match bonus
	if strings.HasPrefix(pathLower, query) {
		score -= 10
	}

	// Shorter paths are better
	score += len(pathRunes) / 5

	// Filename match bonus (match in basename, not directory)
	baseLower := strings.ToLower(baseName(path))
	if strings.Contains(baseLower, query) {
		score -= 5
	}

	return true, score
}

func baseName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
