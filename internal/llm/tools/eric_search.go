package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const ericBaseURL = "https://api.ies.ed.gov/eric/"

// ERIC API response types
type ericResponse struct {
	Response struct {
		NumFound int       `json:"numFound"`
		Docs     []ericDoc `json:"docs"`
	} `json:"response"`
}

type ericDoc struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Author           []string `json:"author"`
	Description      string   `json:"description"`
	PublicationDateY int      `json:"publicationdateyear"`
	Source           string   `json:"source"`
	PublicationType  string   `json:"publicationtype"`
	PeerReviewed     string   `json:"peerreviewed"`
	Descriptor       []string `json:"descriptor"`
	EducationLevel   []string `json:"educationlevel"`
	ISSN             string   `json:"issn"`
	FullText         string   `json:"e_fulltext"` // "Yes" if available
	URL              string   `json:"url"`
}

// ericSearch searches ERIC API and returns formatted results.
func ericSearch(ctx context.Context, query string, limit, offset int) (string, error) {
	result, _, err := ericSearchWithCandidates(ctx, query, limit, offset, nil)
	return result, err
}

func ericSearchWithCandidates(ctx context.Context, query string, limit, offset int, register scholarCandidateRegistrar) (string, map[string]any, error) {
	params := url.Values{
		"search": {query},
		"rows":   {fmt.Sprintf("%d", limit)},
		"start":  {fmt.Sprintf("%d", offset)},
		"format": {"json"},
	}

	metaReq := scholarSearchRequestMeta("eric", query, offset, limit, "format:json")
	body, meta, err := scholarFetchWithPolicy(ctx, ericBaseURL+"?"+params.Encode(), nil, metaReq)
	if err != nil {
		return "", scholarErrorMetadata(meta, err), err
	}

	var ericResp ericResponse
	if err := json.Unmarshal(body, &ericResp); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== ERIC Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d)\n\n", ericResp.Response.NumFound, len(ericResp.Response.Docs))

	if len(ericResp.Response.Docs) == 0 {
		sb.WriteString("No results found.\n")
	}

	for i, doc := range ericResp.Response.Docs {
		abstract := scholarTruncateAbstract(doc.Description)
		pdfURL := ""
		if strings.HasSuffix(strings.ToLower(doc.URL), ".pdf") {
			pdfURL = doc.URL
		}

		if register != nil {
			p := paperResult{
				PaperID:  "ERIC:" + doc.ID,
				Title:    doc.Title,
				Authors:  paperAuthorsFromNames(doc.Author),
				Year:     doc.PublicationDateY,
				Venue:    doc.Source,
				Abstract: doc.Description,
			}
			if pdfURL != "" {
				p.OpenAccessPdf = &openAccessPdf{URL: pdfURL}
			}
			if candidateID := register(i+1+offset, p); candidateID != "" {
				fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
			}
		}

		fmt.Fprintf(&sb, "--- [%d] ---\n", i+1+offset)
		fmt.Fprintf(&sb, "Title: %s\n", doc.Title)
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(doc.Author))
		fmt.Fprintf(&sb, "Year: %d | ERIC ID: %s\n", doc.PublicationDateY, doc.ID)
		if doc.Source != "" {
			fmt.Fprintf(&sb, "Source: %s\n", doc.Source)
		}
		if doc.PublicationType != "" {
			fmt.Fprintf(&sb, "Type: %s", doc.PublicationType)
			if doc.PeerReviewed == "T" {
				sb.WriteString(" (peer-reviewed)")
			}
			sb.WriteString("\n")
		}
		if len(doc.Descriptor) > 0 {
			fmt.Fprintf(&sb, "Descriptors: %s\n", strings.Join(doc.Descriptor, "; "))
		}
		if len(doc.EducationLevel) > 0 {
			fmt.Fprintf(&sb, "Education Level: %s\n", strings.Join(doc.EducationLevel, "; "))
		}
		if doc.FullText == "Yes" || doc.URL != "" {
			ericURL := doc.URL
			if ericURL == "" {
				ericURL = fmt.Sprintf("https://eric.ed.gov/?id=%s", doc.ID)
			}
			fmt.Fprintf(&sb, "Full text: %s\n", ericURL)
		}
		if abstract != "" {
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
		}

		citeKey := generateCiteKeyFromParts(doc.Author, doc.PublicationDateY, doc.Title)
		entryType := "article"
		pubType := strings.ToLower(doc.PublicationType)
		if strings.Contains(pubType, "report") {
			entryType = "techreport"
		} else if strings.Contains(pubType, "book") {
			entryType = "book"
		} else if strings.Contains(pubType, "dissertation") || strings.Contains(pubType, "thesis") {
			entryType = "phdthesis"
		}
		scholarWriteBibHeader(&sb, entryType, citeKey)
		fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(doc.Title))
		fmt.Fprintf(&sb, "  author    = {%s},\n", formatAuthorListBibTeX(doc.Author))
		fmt.Fprintf(&sb, "  year      = {%d}", doc.PublicationDateY)
		if doc.Source != "" && entryType == "article" {
			fmt.Fprintf(&sb, ",\n  journal   = {%s}", doc.Source)
		}
		if doc.ISSN != "" {
			fmt.Fprintf(&sb, ",\n  issn      = {%s}", doc.ISSN)
		}
		fmt.Fprintf(&sb, ",\n  note      = {ERIC ID: %s}", doc.ID)
		sb.WriteString("\n}\n\n")
	}

	return sb.String(), meta, nil
}
