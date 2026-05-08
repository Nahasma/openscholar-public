package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const crossrefBaseURL = "https://api.crossref.org/works"

// CrossRef API response types
type crossrefResponse struct {
	Message struct {
		TotalResults int            `json:"total-results"`
		Items        []crossrefWork `json:"items"`
	} `json:"message"`
}

type crossrefWork struct {
	DOI       string           `json:"DOI"`
	Title     []string         `json:"title"`
	Author    []crossrefAuthor `json:"author"`
	Published struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"published-print"`
	PublishedOnline struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"published-online"`
	ContainerTitle      []string `json:"container-title"`
	Type                string   `json:"type"`
	IsReferencedByCount int      `json:"is-referenced-by-count"`
	Abstract            string   `json:"abstract"`
	Link                []struct {
		URL         string `json:"URL"`
		ContentType string `json:"content-type"`
	} `json:"link"`
}

type crossrefAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

// crossrefSearch searches CrossRef API and returns formatted results.
func crossrefSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := crossrefSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func crossrefSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	params := url.Values{
		"query":  {query},
		"rows":   {fmt.Sprintf("%d", limit)},
		"offset": {fmt.Sprintf("%d", offset)},
		"sort":   {"relevance"},
		"select": {"DOI,title,author,published-print,published-online,container-title,type,is-referenced-by-count,abstract,link"},
	}

	headers := map[string]string{
		"User-Agent": "OpenScholar/1.0 (mailto:openscholar@example.com)",
	}
	queryKey := scholarBuildQueryKey("crossref", "search", map[string]string{
		"query":  query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
		"filter": "sort:relevance",
	})
	body, meta, err := scholarFetchWithPolicy(ctx, crossrefBaseURL+"?"+params.Encode(), headers, scholarRequestMeta{
		Source:   "crossref",
		Action:   "search",
		QueryKey: queryKey,
	})
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var crResp crossrefResponse
	if err := json.Unmarshal(body, &crResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== CrossRef Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d)\n\n", crResp.Message.TotalResults, len(crResp.Message.Items))

	if len(crResp.Message.Items) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, w := range crResp.Message.Items {
		title := ""
		if len(w.Title) > 0 {
			title = w.Title[0]
		}

		var authors []string
		for _, a := range w.Author {
			if a.Given != "" && a.Family != "" {
				authors = append(authors, a.Given+" "+a.Family)
			} else if a.Family != "" {
				authors = append(authors, a.Family)
			}
		}

		year := 0
		if len(w.Published.DateParts) > 0 && len(w.Published.DateParts[0]) > 0 {
			year = w.Published.DateParts[0][0]
		} else if len(w.PublishedOnline.DateParts) > 0 && len(w.PublishedOnline.DateParts[0]) > 0 {
			year = w.PublishedOnline.DateParts[0][0]
		}

		venue := ""
		if len(w.ContainerTitle) > 0 {
			venue = w.ContainerTitle[0]
		}

		abstract := scholarTruncateAbstract(stripHTMLTags(w.Abstract))
		pdfURL := ""
		for _, link := range w.Link {
			contentType := strings.ToLower(link.ContentType)
			linkURL := strings.ToLower(link.URL)
			if strings.Contains(contentType, "pdf") || strings.HasSuffix(linkURL, ".pdf") {
				pdfURL = link.URL
				break
			}
		}

		if register != nil {
			paperID := ""
			if w.DOI != "" {
				paperID = "DOI:" + w.DOI
			}
			p := paperResult{
				PaperID:       paperID,
				Title:         title,
				Authors:       paperAuthorsFromNames(authors),
				Year:          year,
				Venue:         venue,
				Abstract:      stripHTMLTags(w.Abstract),
				CitationCount: w.IsReferencedByCount,
				ExternalIDs: externalIDs{
					DOI: w.DOI,
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
		fmt.Fprintf(&sb, "Title: %s\n", title)
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(authors))
		fmt.Fprintf(&sb, "Year: %d | Citations: %d\n", year, w.IsReferencedByCount)
		if venue != "" {
			fmt.Fprintf(&sb, "Venue: %s\n", venue)
		}
		fmt.Fprintf(&sb, "DOI: %s\n", w.DOI)
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(authors, year, title)
		entryType := crossrefToBibType(w.Type)
		scholarWriteBibHeader(&sb, entryType, citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authors))
		fmt.Fprintf(&sb, "  year      = {%d}", year)
		if venue != "" {
			if entryType == "article" {
				fmt.Fprintf(&sb, ",\n  journal   = {%s}", venue)
			} else {
				fmt.Fprintf(&sb, ",\n  booktitle = {%s}", venue)
			}
		}
		fmt.Fprintf(&sb, ",\n  doi       = {%s}", w.DOI)
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}

func crossrefToBibType(crType string) string {
	switch crType {
	case "journal-article":
		return "article"
	case "proceedings-article", "paper-conference":
		return "inproceedings"
	case "book":
		return "book"
	case "book-chapter":
		return "incollection"
	default:
		return "article"
	}
}

func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			sb.WriteRune(r)
		}
	}
	return cleanWhitespace(sb.String())
}
