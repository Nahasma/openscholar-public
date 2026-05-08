package tools

import "github.com/Nahasma/openscholar-public/internal/config"

// Scholar config helpers — read API keys from config.

func scholarContactEmail() string {
	if c := config.Get(); c != nil && c.Scholar.ContactEmail != "" {
		return c.Scholar.ContactEmail
	}
	return "openscholar@example.com"
}

func scholarOpenAlexKey() string {
	if c := config.Get(); c != nil {
		return c.Scholar.OpenAlexAPIKey
	}
	return ""
}

func scholarNCBIKey() string {
	if c := config.Get(); c != nil {
		return c.Scholar.NCBIAPIKey
	}
	return ""
}

func scholarCoreKey() string {
	if c := config.Get(); c != nil {
		return c.Scholar.CoreAPIKey
	}
	return ""
}

func scholarPatentsViewKey() string {
	if c := config.Get(); c != nil {
		return c.Scholar.PatentsViewAPIKey
	}
	return ""
}

func scholarSemanticScholarKey() string {
	if c := config.Get(); c != nil {
		return c.Scholar.SemanticScholarKey
	}
	return ""
}
