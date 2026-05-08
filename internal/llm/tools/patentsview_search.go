package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const patentsviewBaseURL = "https://search.patentsview.org/api/v1/patent"

// PatentsView API request/response types
type patentsviewQuery struct {
	Q string              `json:"q"`
	F []string            `json:"f"`
	O *patentsviewOptions `json:"o,omitempty"`
}

type patentsviewOptions struct {
	Size int `json:"size"`
	From int `json:"from"`
}

type patentsviewResponse struct {
	Patents   []patentsviewPatent `json:"patents"`
	TotalHits int                 `json:"total_patent_count"`
}

type patentsviewPatent struct {
	PatentID        string `json:"patent_id"`
	PatentTitle     string `json:"patent_title"`
	PatentAbstract  string `json:"patent_abstract"`
	PatentDate      string `json:"patent_date"`
	PatentType      string `json:"patent_type"`
	PatentNumClaims int    `json:"patent_num_claims"`
	Assignees       []struct {
		AssigneeOrganization string `json:"assignee_organization"`
		AssigneeFirstName    string `json:"assignee_first_name"`
		AssigneeLastName     string `json:"assignee_last_name"`
	} `json:"assignees"`
	Inventors []struct {
		InventorFirstName string `json:"inventor_first_name"`
		InventorLastName  string `json:"inventor_last_name"`
	} `json:"inventors"`
}

// patentsviewSearch searches USPTO PatentsView API and returns formatted results.
func patentsviewSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := patentsviewSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func patentsviewSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	apiKey := scholarPatentsViewKey()
	if apiKey == "" {
		meta := map[string]any{
			"source":      "patentsview",
			"action":      "search",
			"query_key":   scholarSearchRequestMeta("patentsview", query, offset, limit, "patent").QueryKey,
			"cache_hit":   false,
			"attempts":    0,
			"retry_count": 0,
			"error_kind":  "missing_api_key",
		}
		return "", meta, fmt.Errorf("PatentsView API key required. Set PATENTSVIEW_API_KEY env var or scholar.patentsViewApiKey in config")
	}

	reqBody := patentsviewQuery{
		Q: query,
		F: []string{
			"patent_id", "patent_title", "patent_abstract", "patent_date",
			"patent_type", "patent_num_claims",
			"assignees.assignee_organization", "assignees.assignee_first_name", "assignees.assignee_last_name",
			"inventors.inventor_first_name", "inventors.inventor_last_name",
		},
		O: &patentsviewOptions{
			Size: limit,
			From: offset,
		},
	}

	metaReq := scholarSearchRequestMeta("patentsview", query, offset, limit, "patent")
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", map[string]any{
			"source":      "patentsview",
			"action":      "search",
			"query_key":   metaReq.QueryKey,
			"cache_hit":   false,
			"attempts":    0,
			"retry_count": 0,
			"error_kind":  "non_retryable",
		}, fmt.Errorf("marshal request: %w", err)
	}

	headers := map[string]string{
		"X-Api-Key": apiKey,
	}
	body, meta, err := scholarPostWithPolicy(ctx, patentsviewBaseURL+"/", bodyBytes, headers, metaReq)
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var pvResp patentsviewResponse
	if err := json.Unmarshal(body, &pvResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== PatentsView Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total patents (showing %d)\n\n", pvResp.TotalHits, len(pvResp.Patents))

	if len(pvResp.Patents) == 0 {
		sb.WriteString("No patents found.\n")
	}

	for i, p := range pvResp.Patents {
		var inventors []string
		for _, inv := range p.Inventors {
			name := strings.TrimSpace(inv.InventorFirstName + " " + inv.InventorLastName)
			if name != "" {
				inventors = append(inventors, name)
			}
		}

		var assignees []string
		for _, a := range p.Assignees {
			if a.AssigneeOrganization != "" {
				assignees = append(assignees, a.AssigneeOrganization)
			} else {
				name := strings.TrimSpace(a.AssigneeFirstName + " " + a.AssigneeLastName)
				if name != "" {
					assignees = append(assignees, name)
				}
			}
		}

		year := 0
		if len(p.PatentDate) >= 4 {
			fmt.Sscanf(p.PatentDate[:4], "%d", &year)
		}

		abstract := scholarTruncateAbstract(p.PatentAbstract)

		inventorStr := scholarFormatAuthors(inventors)
		// PatentsView uses "inventors" not "authors" — adjust suffix label
		if len(inventors) > defaultAuthorMax {
			inventorStr = strings.Join(inventors[:defaultAuthorMax], "; ") + fmt.Sprintf(" ... (%d inventors)", len(inventors))
		}

		if register != nil {
			paper := paperResult{
				PaperID:  "US" + p.PatentID,
				Title:    p.PatentTitle,
				Authors:  paperAuthorsFromNames(inventors),
				Year:     year,
				Venue:    "PatentsView",
				Abstract: p.PatentAbstract,
			}
			if candidateID := register(i+1+offset, paper); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] Patent ---\n", i+1+offset)
		fmt.Fprintf(&sb, "Title: %s\n", p.PatentTitle)
		fmt.Fprintf(&sb, "Inventors: %s\n", inventorStr)
		fmt.Fprintf(&sb, "Patent ID: US%s | Date: %s | Type: %s\n", p.PatentID, p.PatentDate, p.PatentType)
		if len(assignees) > 0 {
			fmt.Fprintf(&sb, "Assignee: %s\n", strings.Join(assignees, "; "))
		}
		if p.PatentNumClaims > 0 {
			fmt.Fprintf(&sb, "Claims: %d\n", p.PatentNumClaims)
		}
		fmt.Fprintf(&sb, "Link: https://patents.google.com/patent/US%s\n", p.PatentID)
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(inventors, year, p.PatentTitle)
		scholarWriteBibHeader(&sb, "misc", citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(p.PatentTitle))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(inventors))
		fmt.Fprintf(&sb, "  year      = {%d},\n", year)
		fmt.Fprintf(&sb, "  note      = {US Patent %s}", p.PatentID)
		if len(assignees) > 0 {
			fmt.Fprintf(&sb, ",\n  howpublished = {%s}", assignees[0])
		}
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}
