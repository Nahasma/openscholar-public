package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const unpaywallBaseURL = "https://api.unpaywall.org/v2"

// Unpaywall API response types
type unpaywallResponse struct {
	DOI            string           `json:"doi"`
	Title          string           `json:"title"`
	Year           int              `json:"year"`
	IsOA           bool             `json:"is_oa"`
	OAStatus       string           `json:"oa_status"`
	BestOALocation *unpaywallOALoc  `json:"best_oa_location"`
	OALocations    []unpaywallOALoc `json:"oa_locations"`
	ZAuthors       []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
	} `json:"z_authors"`
	Genre           string `json:"genre"`
	JournalName     string `json:"journal_name"`
	JournalIsInDOAJ bool   `json:"journal_is_in_doaj"`
}

type unpaywallOALoc struct {
	URLForPdf     string `json:"url_for_pdf"`
	URLForLanding string `json:"url_for_landing_page"`
	HostType      string `json:"host_type"`
	License       string `json:"license"`
	Version       string `json:"version"`
}

// unpaywallLookup queries Unpaywall for OA status of a DOI and returns formatted result.
func unpaywallLookup(ctx context.Context, doi string) (string, error) {
	result, _, err := unpaywallLookupWithCandidate(ctx, doi, nil)
	return result, err
}

func unpaywallLookupWithCandidate(ctx context.Context, doi string, register scholarCandidateRegistrar) (string, map[string]any, error) {
	email := scholarContactEmail()

	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")

	reqURL := fmt.Sprintf("%s/%s?email=%s", unpaywallBaseURL, doi, email)
	metaReq := scholarRequestMeta{
		Source: "unpaywall",
		Action: "search",
		QueryKey: scholarBuildQueryKey("unpaywall", "search", map[string]string{
			"query":  doi,
			"id":     doi,
			"offset": "0",
			"limit":  "1",
			"filter": "doi",
		}),
	}
	body, meta, err := scholarFetchWithPolicy(ctx, reqURL, nil, metaReq)
	if err != nil {
		var httpErr *scholarHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			noResultsMeta := scholarErrorMetadata(meta, err)
			noResultsMeta["error_kind"] = "no_results"
			return fmt.Sprintf("Unpaywall: DOI %s not found.\n", doi), noResultsMeta, nil
		}
		return "", scholarErrorMetadata(meta, err), err
	}

	var result unpaywallResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", meta, fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Unpaywall: DOI %s ===\n", doi)
	if result.Title != "" {
		fmt.Fprintf(&sb, "Title: %s\n", result.Title)
	}

	var authors []string
	for _, a := range result.ZAuthors {
		if a.Given != "" && a.Family != "" {
			authors = append(authors, a.Given+" "+a.Family)
		} else if a.Family != "" {
			authors = append(authors, a.Family)
		}
	}
	if len(authors) > 0 {
		fmt.Fprintf(&sb, "Authors: %s\n", scholarFormatAuthors(authors))
	}

	pdfURL := unpaywallBestPDF(result)
	if register != nil {
		p := paperResult{
			PaperID: "DOI:" + firstNonEmpty(result.DOI, doi),
			Title:   result.Title,
			Authors: paperAuthorsFromNames(authors),
			Year:    result.Year,
			Venue:   result.JournalName,
			ExternalIDs: externalIDs{
				DOI: firstNonEmpty(result.DOI, doi),
			},
		}
		if pdfURL != "" {
			p.OpenAccessPdf = &openAccessPdf{URL: pdfURL}
		}
		if candidateID := register(1, p); candidateID != "" {
			fmt.Fprintf(&sb, "CandidateID: %s\n", candidateID)
		}
	}

	if result.Year > 0 {
		fmt.Fprintf(&sb, "Year: %d\n", result.Year)
	}
	if result.JournalName != "" {
		fmt.Fprintf(&sb, "Journal: %s\n", result.JournalName)
	}

	fmt.Fprintf(&sb, "OA Status: %s (is_oa: %t)\n", result.OAStatus, result.IsOA)
	if result.JournalIsInDOAJ {
		sb.WriteString("Journal is in DOAJ: yes\n")
	}

	if result.BestOALocation != nil {
		loc := result.BestOALocation
		if loc.URLForPdf != "" {
			fmt.Fprintf(&sb, "Best PDF: %s\n", loc.URLForPdf)
		}
		if loc.URLForLanding != "" {
			fmt.Fprintf(&sb, "Landing page: %s\n", loc.URLForLanding)
		}
		if loc.License != "" {
			fmt.Fprintf(&sb, "License: %s\n", loc.License)
		}
		fmt.Fprintf(&sb, "Host: %s | Version: %s\n", loc.HostType, loc.Version)
	} else if result.IsOA && len(result.OALocations) > 0 {
		loc := result.OALocations[0]
		if loc.URLForPdf != "" {
			fmt.Fprintf(&sb, "PDF: %s\n", loc.URLForPdf)
		}
	} else {
		sb.WriteString("No open access version found.\n")
	}

	return sb.String(), meta, nil
}

func unpaywallBestPDF(result unpaywallResponse) string {
	if result.BestOALocation != nil && result.BestOALocation.URLForPdf != "" {
		return result.BestOALocation.URLForPdf
	}
	for _, loc := range result.OALocations {
		if loc.URLForPdf != "" {
			return loc.URLForPdf
		}
	}
	return ""
}
