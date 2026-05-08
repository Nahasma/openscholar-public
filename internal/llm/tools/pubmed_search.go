package tools

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	pubmedSearchURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"
	pubmedFetchURL  = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
)

// PubMed eSearch response
type pubmedSearchResponse struct {
	IDList []string `json:"idlist"`
	Count  string   `json:"count"`
}

type pubmedESearchResult struct {
	Result pubmedSearchResponse `json:"esearchresult"`
}

// PubMed eFetch XML response
type pubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID    string `xml:"PMID"`
		Article struct {
			ArticleTitle string `xml:"ArticleTitle"`
			Abstract     struct {
				AbstractText string `xml:"AbstractText"`
			} `xml:"Abstract"`
			AuthorList struct {
				Authors []struct {
					LastName string `xml:"LastName"`
					ForeName string `xml:"ForeName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
			Journal struct {
				Title        string `xml:"Title"`
				JournalIssue struct {
					PubDate struct {
						Year string `xml:"Year"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
			ELocationID []struct {
				EIdType string `xml:"EIdType,attr"`
				Value   string `xml:",chardata"`
			} `xml:"ELocationID"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
}

// pubmedSearch searches PubMed and returns formatted results.
func pubmedSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := pubmedSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func pubmedSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	// Step 1: eSearch to get PMIDs
	searchParams := url.Values{
		"db":       {"pubmed"},
		"term":     {query},
		"retmax":   {fmt.Sprintf("%d", limit)},
		"retstart": {fmt.Sprintf("%d", offset)},
		"retmode":  {"json"},
		"sort":     {"relevance"},
	}

	searchQueryKey := scholarBuildQueryKey("pubmed", "search", map[string]string{
		"query":  query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
		"filter": "esearch",
	})
	searchBody, meta, err := scholarFetchWithPolicy(ctx, pubmedSearchURL+"?"+searchParams.Encode(), nil, scholarRequestMeta{
		Source:   "pubmed",
		Action:   "search",
		QueryKey: searchQueryKey,
	})
	if err != nil {
		return "", scholarErrorMetadata(meta, err), fmt.Errorf("search request: %w", err)
	}

	var eSearchResult pubmedESearchResult
	if err := json.Unmarshal(searchBody, &eSearchResult); err != nil {
		return "", meta, fmt.Errorf("parse search response: %w", err)
	}

	pmids := eSearchResult.Result.IDList
	if len(pmids) == 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "=== PubMed Search: %q ===\n", query)
		sb.WriteString("No results found.\n")
		return sb.String(), meta, nil
	}

	// Step 2: eFetch to get article details
	fetchParams := url.Values{
		"db":      {"pubmed"},
		"id":      {strings.Join(pmids, ",")},
		"retmode": {"xml"},
		"rettype": {"full"},
	}

	// Rate limit: 3 requests/second for PubMed without API key
	time.Sleep(350 * time.Millisecond)

	fetchQueryKey := scholarBuildQueryKey("pubmed", "search", map[string]string{
		"query":  query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
		"filter": "efetch:" + strings.Join(pmids, ","),
	})
	fetchBody, fetchMeta, err := scholarFetchWithPolicy(ctx, pubmedFetchURL+"?"+fetchParams.Encode(), nil, scholarRequestMeta{
		Source:   "pubmed",
		Action:   "search",
		QueryKey: fetchQueryKey,
	})
	if err != nil {
		return "", scholarErrorMetadata(fetchMeta, err), fmt.Errorf("fetch request: %w", err)
	}

	var articleSet pubmedArticleSet
	if err := xml.Unmarshal(fetchBody, &articleSet); err != nil {
		return "", fetchMeta, fmt.Errorf("parse articles: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== PubMed Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %s total results (showing %d)\n\n", eSearchResult.Result.Count, len(articleSet.Articles))

	for i, art := range articleSet.Articles {
		mc := art.MedlineCitation
		title := mc.Article.ArticleTitle
		journal := mc.Article.Journal.Title
		pmid := mc.PMID

		year := 0
		fmt.Sscanf(mc.Article.Journal.JournalIssue.PubDate.Year, "%d", &year)

		var authors []string
		for _, a := range mc.Article.AuthorList.Authors {
			if a.ForeName != "" && a.LastName != "" {
				authors = append(authors, a.ForeName+" "+a.LastName)
			} else if a.LastName != "" {
				authors = append(authors, a.LastName)
			}
		}

		doi := ""
		for _, eid := range mc.Article.ELocationID {
			if eid.EIdType == "doi" {
				doi = eid.Value
				break
			}
		}

		abstract := scholarTruncateAbstract(mc.Article.Abstract.AbstractText)

		if register != nil {
			p := paperResult{
				PaperID:  "PMID:" + pmid,
				Title:    title,
				Authors:  paperAuthorsFromNames(authors),
				Year:     year,
				Venue:    journal,
				Abstract: mc.Article.Abstract.AbstractText,
				ExternalIDs: externalIDs{
					DOI: doi,
				},
			}
			if candidateID := register(i+1+offset, p); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] ---\n", i+1+offset)
		fmt.Fprintf(&sb, "Title: %s\n", title)
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(authors))
		fmt.Fprintf(&sb, "Year: %d | PMID: %s\n", year, pmid)
		if journal != "" {
			fmt.Fprintf(&sb, "Journal: %s\n", journal)
		}
		if doi != "" {
			fmt.Fprintf(&sb, "DOI: %s\n", doi)
		}
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(authors, year, title)
		scholarWriteBibHeader(&sb, "article", citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(authors))
		fmt.Fprintf(&sb, "  year      = {%d}", year)
		if journal != "" {
			fmt.Fprintf(&sb, ",\n  journal   = {%s}", journal)
		}
		if doi != "" {
			fmt.Fprintf(&sb, ",\n  doi       = {%s}", doi)
		}
		fmt.Fprintf(&sb, ",\n  pmid      = {%s}", pmid)
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), fetchMeta, nil
}
