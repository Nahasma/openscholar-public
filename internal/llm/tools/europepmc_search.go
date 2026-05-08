package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const europePMCBaseURL = "https://www.ebi.ac.uk/europepmc/webservices/rest"

// Europe PMC API response types

type europePMCResultList struct {
	Result []europePMCResult `json:"result"`
}

type europePMCSearchResponse struct {
	HitCount   int                 `json:"hitCount"`
	ResultList europePMCResultList `json:"resultList"`
}

type europePMCResult struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	PMID            string `json:"pmid"`
	PMCID           string `json:"pmcid"`
	DOI             string `json:"doi"`
	Title           string `json:"title"`
	AuthorString    string `json:"authorString"`
	JournalTitle    string `json:"journalTitle"`
	PubYear         string `json:"pubYear"`
	AbstractText    string `json:"abstractText"`
	CitedByCount    int    `json:"citedByCount"`
	IsOpenAccess    string `json:"isOpenAccess"`
	HasFullText     string `json:"inEPMC"`
	PubType         string `json:"pubType"`
	FullTextURLList *struct {
		FullTextURL []struct {
			URL           string `json:"url"`
			DocumentStyle string `json:"documentStyle"`
			Availability  string `json:"availabilityCode"`
		} `json:"fullTextUrl"`
	} `json:"fullTextUrlList"`
}

// europePMCSearch searches Europe PMC API and returns formatted results.
func europePMCSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := europePMCSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func europePMCSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	page := offset/limit + 1
	if page < 1 {
		page = 1
	}

	params := url.Values{
		"query":      {query},
		"pageSize":   {fmt.Sprintf("%d", limit)},
		"page":       {fmt.Sprintf("%d", page)},
		"format":     {"json"},
		"resultType": {"core"},
	}

	metaReq := scholarSearchRequestMeta("europepmc", query, offset, limit, "resultType:core")
	body, meta, err := scholarFetchWithPolicy(ctx, europePMCBaseURL+"/search?"+params.Encode(), nil, metaReq)
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var searchResp europePMCSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	results := searchResp.ResultList.Result

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Europe PMC Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d)\n\n", searchResp.HitCount, len(results))

	if len(results) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, r := range results {
		authorStr := r.AuthorString
		if len(authorStr) > 120 {
			authorStr = authorStr[:120] + "..."
		}

		year := 0
		fmt.Sscanf(r.PubYear, "%d", &year)

		abstract := scholarTruncateAbstract(r.AbstractText)

		fullTextURL := ""
		pdfURL := ""
		if r.FullTextURLList != nil {
			for _, u := range r.FullTextURLList.FullTextURL {
				if u.DocumentStyle == "pdf" {
					fullTextURL = u.URL
					pdfURL = u.URL
					break
				}
			}
			if fullTextURL == "" && len(r.FullTextURLList.FullTextURL) > 0 {
				fullTextURL = r.FullTextURLList.FullTextURL[0].URL
			}
		}

		if register != nil {
			paperID := ""
			for _, id := range []string{r.PMID, r.PMCID, r.ID} {
				id = strings.TrimSpace(id)
				if id != "" {
					paperID = "EuropePMC:" + id
					break
				}
			}
			p := paperResult{
				PaperID:       paperID,
				Title:         r.Title,
				Authors:       paperAuthorsFromNames(parseEuropePMCAuthors(r.AuthorString)),
				Year:          year,
				Venue:         r.JournalTitle,
				Abstract:      r.AbstractText,
				CitationCount: r.CitedByCount,
				ExternalIDs: externalIDs{
					DOI: r.DOI,
				},
			}
			if pdfURL != "" {
				p.OpenAccessPdf = &openAccessPdf{URL: pdfURL}
			}
			if candidateID := register(i+1+offset, p); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] ---\n", i+1+offset)
		fmt.Fprintf(&sb, "Title: %s\n", r.Title)
		fmt.Fprintf(&sb, "Authors: %s\n", authorStr)
		fmt.Fprintf(&sb, "Year: %d | Citations: %d\n", year, r.CitedByCount)
		if r.JournalTitle != "" {
			fmt.Fprintf(&sb, "Journal: %s\n", r.JournalTitle)
		}
		if r.PMID != "" {
			fmt.Fprintf(&sb, "PMID: %s", r.PMID)
			if r.PMCID != "" {
				fmt.Fprintf(&sb, " | PMCID: %s", r.PMCID)
			}
			sb.WriteString("\n")
		}
		if r.DOI != "" {
			fmt.Fprintf(&sb, "DOI: %s\n", r.DOI)
		}
		if r.IsOpenAccess == "Y" {
			sb.WriteString("Open Access: Yes\n")
		}
		if fullTextURL != "" {
			fmt.Fprintf(&sb, "Full text: %s\n", fullTextURL)
		}
		if r.PubType != "" {
			fmt.Fprintf(&sb, "Type: %s\n", r.PubType)
		}
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		authorNames := parseEuropePMCAuthors(r.AuthorString)
		citeKey := generateCiteKeyFromParts(authorNames, year, r.Title)
		scholarWriteBibHeader(&sb, "article", citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(r.Title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authorNames))
		fmt.Fprintf(&sb, "  year      = {%d}", year)
		if r.JournalTitle != "" {
			fmt.Fprintf(&sb, ",\n  journal   = {%s}", r.JournalTitle)
		}
		if r.DOI != "" {
			fmt.Fprintf(&sb, ",\n  doi       = {%s}", r.DOI)
		}
		if r.PMID != "" {
			fmt.Fprintf(&sb, ",\n  pmid      = {%s}", r.PMID)
		}
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}

// parseEuropePMCAuthors parses "Smith J, Doe AB, ..." into a slice of names.
func parseEuropePMCAuthors(authorString string) []string {
	if authorString == "" {
		return nil
	}
	authorString = strings.TrimRight(authorString, ".")
	parts := strings.Split(authorString, ", ")
	var authors []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			authors = append(authors, p)
		}
	}
	return authors
}
