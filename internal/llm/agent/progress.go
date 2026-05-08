package agent

import "strings"

func classifyRoundProgress(state *LoopState, outcomes []ToolOutcome) RoundProgress {
	rp := RoundProgress{ToolCalls: len(outcomes)}
	art := make(map[string]struct{})
	lowValueCount := 0
	newEvidenceCount := 0
	for _, out := range outcomes {
		if out.IsError {
			rp.FailedTools++
		} else {
			rp.SuccessfulTools++
		}
		if out.DurableProgress {
			rp.DurableProgress = true
		}
		rp.ResultBytes += out.ResultBytes
		for _, p := range out.ArtifactPaths {
			if p == "" {
				continue
			}
			if _, ok := art[p]; !ok {
				art[p] = struct{}{}
				rp.ArtifactPaths = append(rp.ArtifactPaths, p)
			}
		}
		if state != nil {
			for _, key := range out.EvidenceKeys {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				if state.SeenEvidence[key] == 0 {
					newEvidenceCount++
				}
				state.SeenEvidence[key]++
			}
			if out.TargetKey != "" {
				state.TargetCounts[out.TargetKey]++
			}
			if out.OutcomeHash != "" {
				state.OutcomeCounts[out.OutcomeHash]++
				if out.TargetKey != "" {
					state.OutcomeCounts[out.TargetKey+"|"+out.OutcomeHash]++
				}
			}
			if isSearchFamilyTool(out.ToolName) {
				state.SearchToolCalls++
			}
			if out.IsError && out.Signature != "" {
				state.FailureCounts[out.Signature]++
			}
		}
		if isLowValueInspectionOutcome(out) {
			lowValueCount++
		}
	}
	if newEvidenceCount > 0 {
		rp.NewEvidenceCount = newEvidenceCount
		rp.DurableProgress = true
	}
	if len(rp.ArtifactPaths) > 0 {
		rp.DurableProgress = true
	}
	rp.LowValueInspection = rp.ToolCalls > 0 && lowValueCount == rp.ToolCalls
	return rp
}

func isLowValueInspectionOutcome(out ToolOutcome) bool {
	if isNonDurableProgressKind(out.ProgressKind) && !out.DurableProgress {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(out.ToolName))
	switch name {
	case "bash":
		q := strings.ToLower(strings.TrimSpace(out.QueryKey))
		if strings.Contains(q, "--help") || strings.Contains(q, " -h") || strings.Contains(q, "--version") || strings.Contains(q, " which ") || strings.HasPrefix(q, "which ") || strings.HasPrefix(q, "ls ") || q == "ls" {
			return true
		}
	}
	if out.ContentEmpty {
		return true
	}
	if !out.DurableProgress {
		class := strings.ToLower(strings.TrimSpace(out.ContentClass))
		pg := strings.ToLower(strings.TrimSpace(out.ProgressKind))
		if pg == "fetched_page" && (class == "list_page" || class == "search_page" || class == "index_page") {
			return true
		}
	}
	switch name {
	case "view", "glob", "grep", "read":
		return !out.IsError && out.ResultBytes <= 64 && len(out.ArtifactPaths) == 0
	}
	if name == "" && !out.IsError && out.ResultBytes <= 64 && len(out.ArtifactPaths) == 0 {
		return true
	}
	return false
}

func isSearchFamilyTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "websearch", "webfetch", "scholarsearch", "kbsearch", "kbquery":
		return true
	default:
		return false
	}
}
