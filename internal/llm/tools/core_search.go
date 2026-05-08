package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const coreBaseURL = "https://api.core.ac.uk/v3"

// CORE API response types
type coreSearchResponse struct {
	TotalHits int        `json:"totalHits"`
	Results   []coreWork `json:"results"`
}

type coreWork struct {
	ID                 int           `json:"id"`
	Title              string        `json:"title"`
	Authors            []coreAuthor  `json:"authors"`
	YearPublished      int           `json:"yearPublished"`
	Abstract           string        `json:"abstract"`
	DOI                string        `json:"doi"`
	DownloadURL        string        `json:"downloadUrl"`
	SourceFulltextUrls []string      `json:"sourceFulltextUrls"`
	Publisher          string        `json:"publisher"`
	Journals           []coreJournal `json:"journals"`
	DocumentType       string        `json:"documentType"`
	Language           *coreLang     `json:"language"`
	CitationCount      int           `json:"citationCount"`
}

type coreAuthor struct {
	Name string `json:"name"`
}

type coreJournal struct {
	Title string `json:"title"`
}

type coreLang struct {
	Code string `json:"code"`
}

// coreSearch searches CORE API and returns formatted results.
func coreSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := coreSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func coreSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	apiKey := scholarCoreKey()

	params := url.Values{
		"q":      {query},
		"limit":  {fmt.Sprintf("%d", limit)},
		"offset": {fmt.Sprintf("%d", offset)},
	}

	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	metaReq := scholarSearchRequestMeta("core", query, offset, limit, "works")
	body, meta, err := scholarFetchWithPolicy(ctx, coreBaseURL+"/search/works?"+params.Encode(), headers, metaReq)
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var searchResp coreSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== CORE Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d)\n\n", searchResp.TotalHits, len(searchResp.Results))

	if len(searchResp.Results) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, w := range searchResp.Results {
		var authors []string
		for _, a := range w.Authors {
			authors = append(authors, a.Name)
		}

		venue := ""
		if len(w.Journals) > 0 && w.Journals[0].Title != "" {
			venue = w.Journals[0].Title
		} else if w.Publisher != "" {
			venue = w.Publisher
		}

		doi := strings.TrimPrefix(w.DOI, "https://doi.org/")
		abstract := scholarTruncateAbstract(w.Abstract)

		docType := w.DocumentType
		if docType == "" {
			docType = "article"
		}

		if register != nil {
			p := paperResult{
				PaperID:       fmt.Sprintf("CORE:%d", w.ID),
				Title:         w.Title,
				Authors:       paperAuthorsFromNames(authors),
				Year:          w.YearPublished,
				Venue:         venue,
				Abstract:      w.Abstract,
				CitationCount: w.CitationCount,
				ExternalIDs: externalIDs{
					DOI: doi,
				},
			}
			if w.DownloadURL != "" {
				p.OpenAccessPdf = &openAccessPdf{URL: w.DownloadURL}
			}
			if candidateID := register(i+1+offset, p); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] ---\n", i+1+offset)
		fmt.Fprintf(&sb, "Title: %s\n", w.Title)
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(authors))
		fmt.Fprintf(&sb, "Year: %d", w.YearPublished)
		if w.CitationCount > 0 {
			fmt.Fprintf(&sb, " | Citations: %d", w.CitationCount)
		}
		fmt.Fprintf(&sb, " | Type: %s\n", docType)
		if venue != "" {
			fmt.Fprintf(&sb, "Venue: %s\n", venue)
		}
		if doi != "" {
			fmt.Fprintf(&sb, "DOI: %s\n", doi)
		}
		if w.DownloadURL != "" {
			fmt.Fprintf(&sb, "PDF: %s\n", w.DownloadURL)
		} else if len(w.SourceFulltextUrls) > 0 {
			fmt.Fprintf(&sb, "Full text: %s\n", w.SourceFulltextUrls[0])
		}
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(authors, w.YearPublished, w.Title)
		entryType := "article"
		if docType == "thesis" || docType == "dissertation" {
			entryType = "phdthesis"
		}
		scholarWriteBibHeader(&sb, entryType, citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(w.Title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authors))
		fmt.Fprintf(&sb, "  year      = {%d}", w.YearPublished)
		if venue != "" {
			fmt.Fprintf(&sb, ",\n  journal   = {%s}", venue)
		}
		if doi != "" {
			fmt.Fprintf(&sb, ",\n  doi       = {%s}", doi)
		}
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}
