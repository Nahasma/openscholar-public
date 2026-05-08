package tools

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"strings"
)

const arxivBaseURL = "http://export.arxiv.org/api/query"

// arXiv API response types (Atom XML)
type arxivFeed struct {
	XMLName      xml.Name     `xml:"feed"`
	TotalResults int          `xml:"totalResults"`
	Entries      []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	Updated   string        `xml:"updated"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink   `xml:"link"`
	Category  []arxivCat    `xml:"category"`
	Comment   string        `xml:"comment"`
	DOI       string        `xml:"doi"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
	Rel  string `xml:"rel,attr"`
}

type arxivCat struct {
	Term string `xml:"term,attr"`
}

// arxivSearch searches arXiv API and returns formatted results.
func arxivSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := arxivSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func arxivSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	searchQuery := fmt.Sprintf("all:%s", query)
	if idQuery := scholarParseArxivIDQuery(query); idQuery != "" {
		searchQuery = fmt.Sprintf("id:%s", idQuery)
	}
	params := url.Values{
		"search_query": {searchQuery},
		"start":        {fmt.Sprintf("%d", offset)},
		"max_results":  {fmt.Sprintf("%d", limit)},
		"sortBy":       {"relevance"},
	}

	queryKey := scholarBuildQueryKey("arxiv", "search", map[string]string{
		"query":  query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
		"filter": searchQuery,
	})
	body, meta, err := scholarFetchWithPolicy(ctx, arxivBaseURL+"?"+params.Encode(), nil, scholarRequestMeta{
		Source:   "arxiv",
		Action:   "search",
		QueryKey: queryKey,
	})
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return "", meta, fmt.Errorf("parse XML: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== arXiv Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d, offset %d)\n\n", feed.TotalResults, len(feed.Entries), offset)

	if len(feed.Entries) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, e := range feed.Entries {
		arxivID := extractArxivID(e.ID)
		year := extractYear(e.Published)

		var authors []string
		for _, a := range e.Authors {
			authors = append(authors, a.Name)
		}

		var cats []string
		for _, c := range e.Category {
			cats = append(cats, c.Term)
		}

		title := cleanWhitespace(html.UnescapeString(e.Title))
		fullAbstract := cleanWhitespace(html.UnescapeString(e.Summary))
		abstract := scholarTruncateAbstract(fullAbstract)
		pdfURL := ""
		for _, l := range e.Links {
			if l.Type == "application/pdf" {
				pdfURL = l.Href
				break
			}
		}

		if register != nil {
			p := paperResult{
				PaperID:  "ArXiv:" + arxivID,
				Title:    title,
				Authors:  arxivPaperAuthors(e.Authors),
				Year:     year,
				Venue:    "arXiv",
				Abstract: fullAbstract,
				ExternalIDs: externalIDs{
					DOI:   e.DOI,
					ArXiv: arxivID,
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
		fmt.Fprintf(&sb, "Year: %d | ArXiv: %s\n", year, arxivID)
		if len(cats) > 0 {
			fmt.Fprintf(&sb, "Categories: %s\n", strings.Join(cats, ", "))
		}
		if e.DOI != "" {
			fmt.Fprintf(&sb, "DOI: %s\n", e.DOI)
		}
		if pdfURL != "" {
			fmt.Fprintf(&sb, "PDF: %s\n", pdfURL)
		}
		fmt.Fprintf(&sb, "Abstract: %s\n", abstract)

		citeKey := generateCiteKeyFromParts(authors, year, title)
		scholarWriteBibHeader(&sb, "misc", citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authors))
		fmt.Fprintf(&sb, "  year      = {%d},\n", year)
		fmt.Fprintf(&sb, "  eprint    = {%s},\n", arxivID)
		fmt.Fprintf(&sb, "  archiveprefix = {arXiv}")
		if e.DOI != "" {
			fmt.Fprintf(&sb, ",\n  doi       = {%s}", e.DOI)
		}
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}

func arxivPaperAuthors(authors []arxivAuthor) []authorResult {
	result := make([]authorResult, 0, len(authors))
	for _, a := range authors {
		result = append(result, authorResult{Name: a.Name})
	}
	return result
}

func extractArxivID(fullURL string) string {
	// "http://arxiv.org/abs/2301.12345v1" → "2301.12345"
	parts := strings.Split(fullURL, "/abs/")
	if len(parts) == 2 {
		id := parts[1]
		if idx := strings.LastIndex(id, "v"); idx > 0 {
			id = id[:idx]
		}
		return id
	}
	return fullURL
}

func extractYear(published string) int {
	if len(published) >= 4 {
		var y int
		fmt.Sscanf(published[:4], "%d", &y)
		return y
	}
	return 0
}

func cleanWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func generateCiteKeyFromParts(authors []string, year int, title string) string {
	lastName := "unknown"
	if len(authors) > 0 {
		lastName = extractLastName(authors[0])
	}
	contentWord := extractFirstContentWord(title)
	if year == 0 {
		year = 9999
	}
	return fmt.Sprintf("%s%d%s", lastName, year, contentWord)
}

func formatAuthorListBibTeX(authors []string) string {
	if len(authors) == 0 {
		return "Unknown"
	}
	var parts []string
	for i, name := range authors {
		if i >= 5 {
			parts = append(parts, "others")
			break
		}
		parts = append(parts, flipAuthorName(name))
	}
	return strings.Join(parts, " and ")
}
