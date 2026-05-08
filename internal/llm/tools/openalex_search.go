package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const openAlexBaseURL = "https://api.openalex.org"

// OpenAlex API response types
type openAlexSearchResponse struct {
	Results []openAlexWork `json:"results"`
	Meta    struct {
		Count int `json:"count"`
	} `json:"meta"`
}

type openAlexWork struct {
	ID                    string           `json:"id"`
	Title                 string           `json:"title"`
	PublicationYear       int              `json:"publication_year"`
	CitedByCount          int              `json:"cited_by_count"`
	DOI                   string           `json:"doi"`
	Authorships           []openAlexAuth   `json:"authorships"`
	PrimaryLocation       *openAlexLoc     `json:"primary_location"`
	OpenAccess            *openAlexOA      `json:"open_access"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
}

type openAlexAuth struct {
	Author struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

type openAlexLoc struct {
	Source *struct {
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	} `json:"source"`
}

type openAlexOA struct {
	IsOA  bool   `json:"is_oa"`
	OAURL string `json:"oa_url"`
}

// openAlexSearch searches OpenAlex API and returns formatted results.
func openAlexSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := openAlexSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func openAlexSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	params := url.Values{
		"search":   {query},
		"per_page": {fmt.Sprintf("%d", limit)},
		"page":     {fmt.Sprintf("%d", offset/limit+1)},
		"sort":     {"relevance_score:desc"},
		"select":   {"id,title,publication_year,cited_by_count,doi,authorships,primary_location,open_access,abstract_inverted_index"},
	}

	email := scholarContactEmail()
	if apiKey := scholarOpenAlexKey(); apiKey != "" {
		params.Set("api_key", apiKey)
	}
	if email != "" {
		params.Set("mailto", email)
	}

	headers := map[string]string{
		"User-Agent": fmt.Sprintf("OpenScholar/2.0 (mailto:%s)", email),
	}
	queryKey := scholarBuildQueryKey("openalex", "search", map[string]string{
		"query":  query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
		"filter": "sort:relevance_score:desc",
	})
	body, meta, err := scholarFetchWithPolicy(ctx, openAlexBaseURL+"/works?"+params.Encode(), headers, scholarRequestMeta{
		Source:   "openalex",
		Action:   "search",
		QueryKey: queryKey,
	})
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var searchResp openAlexSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== OpenAlex Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d)\n\n", searchResp.Meta.Count, len(searchResp.Results))

	if len(searchResp.Results) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, w := range searchResp.Results {
		var authors []string
		for _, a := range w.Authorships {
			authors = append(authors, a.Author.DisplayName)
		}

		venue := ""
		if w.PrimaryLocation != nil && w.PrimaryLocation.Source != nil {
			venue = w.PrimaryLocation.Source.DisplayName
		}

		doi := strings.TrimPrefix(w.DOI, "https://doi.org/")
		fullAbstract := reconstructAbstract(w.AbstractInvertedIndex)
		abstract := scholarTruncateAbstract(fullAbstract)

		if register != nil {
			p := paperResult{
				PaperID:       w.ID,
				Title:         w.Title,
				Authors:       paperAuthorsFromNames(authors),
				Year:          w.PublicationYear,
				Venue:         venue,
				Abstract:      fullAbstract,
				CitationCount: w.CitedByCount,
				ExternalIDs: externalIDs{
					DOI: doi,
				},
			}
			if w.OpenAccess != nil && w.OpenAccess.OAURL != "" {
				p.OpenAccessPdf = &openAccessPdf{URL: w.OpenAccess.OAURL}
			}
			if candidateID := register(i+1+offset, p); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] ---\n", i+1)
		fmt.Fprintf(&sb, "Title: %s\n", w.Title)
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(authors))
		fmt.Fprintf(&sb, "Year: %d | Citations: %d\n", w.PublicationYear, w.CitedByCount)
		if venue != "" {
			fmt.Fprintf(&sb, "Venue: %s\n", venue)
		}
		if doi != "" {
			fmt.Fprintf(&sb, "DOI: %s\n", doi)
		}
		if w.OpenAccess != nil && w.OpenAccess.OAURL != "" {
			fmt.Fprintf(&sb, "PDF: %s\n", w.OpenAccess.OAURL)
		}
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(authors, w.PublicationYear, w.Title)
		entryType := "article"
		if w.PrimaryLocation != nil && w.PrimaryLocation.Source != nil && w.PrimaryLocation.Source.Type == "repository" {
			entryType = "misc"
		}
		scholarWriteBibHeader(&sb, entryType, citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(w.Title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authors))
		fmt.Fprintf(&sb, "  year      = {%d}", w.PublicationYear)
		if venue != "" {
			if entryType == "article" {
				fmt.Fprintf(&sb, ",\n  journal   = {%s}", venue)
			} else {
				fmt.Fprintf(&sb, ",\n  booktitle = {%s}", venue)
			}
		}
		if doi != "" {
			fmt.Fprintf(&sb, ",\n  doi       = {%s}", doi)
		}
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}

// reconstructAbstract rebuilds the abstract from OpenAlex's inverted index format.
func reconstructAbstract(invertedIndex map[string][]int) string {
	if len(invertedIndex) == 0 {
		return ""
	}

	maxPos := 0
	for _, positions := range invertedIndex {
		for _, p := range positions {
			if p > maxPos {
				maxPos = p
			}
		}
	}

	words := make([]string, maxPos+1)
	for word, positions := range invertedIndex {
		for _, p := range positions {
			if p <= maxPos {
				words[p] = word
			}
		}
	}

	return strings.Join(words, " ")
}
