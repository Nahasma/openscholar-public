package modules

import (
	"fmt"
	"strings"
)

// ProfileData contains the user profile fields for prompt rendering.
// Decoupled from the init package to avoid circular imports.
type ProfileData struct {
	Domain        string
	Subdomain     string
	Language      string
	Role          string
	PubLevel      string
	ResearchType  string
	Weaknesses    string // comma-separated
	FeedbackStyle string
	SpecialNeeds  string
	LaTeXLevel    string
	Collaboration string
}

// NewProfileModule creates a prompt module that injects the user profile.
// Priority 1: appears right after base (0), before tools (5).
// Returns a DynamicBaseModule so the provider can skip caching this block.
func NewProfileModule(data ProfileData) DynamicBaseModule {
	if data.Domain == "" {
		return NewDynamicBaseModule("profile", "", 1)
	}

	var sb strings.Builder
	sb.WriteString("# User Profile\n")
	fmt.Fprintf(&sb, "- Research domain: %s / %s\n", data.Domain, data.Subdomain)
	fmt.Fprintf(&sb, "- Academic role: %s\n", data.Role)
	fmt.Fprintf(&sb, "- Writing language: %s\n", data.Language)
	fmt.Fprintf(&sb, "- Target publication level: %s\n", data.PubLevel)

	if data.ResearchType != "" {
		fmt.Fprintf(&sb, "- Research type: %s\n", data.ResearchType)
	}
	if data.Weaknesses != "" {
		fmt.Fprintf(&sb, "- Writing areas needing extra help: %s\n", data.Weaknesses)
	}
	if data.FeedbackStyle != "" {
		fmt.Fprintf(&sb, "- Feedback style preference: %s\n", data.FeedbackStyle)
	}
	if data.LaTeXLevel != "" {
		fmt.Fprintf(&sb, "- LaTeX experience: %s\n", data.LaTeXLevel)
	}
	if data.Collaboration != "" {
		fmt.Fprintf(&sb, "- Collaboration style: %s\n", data.Collaboration)
	}
	if data.SpecialNeeds != "" {
		fmt.Fprintf(&sb, "- Special needs: %s\n", data.SpecialNeeds)
	}

	return NewDynamicBaseModule("profile", sb.String(), 1)
}
