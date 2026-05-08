package initwizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/fileop"
	"gopkg.in/yaml.v3"
)

// UserProfile stores the user's research profile collected during Soft Init.
type UserProfile struct {
	// Concise mode fields (always present)
	Domain    string `yaml:"domain"`
	Subdomain string `yaml:"subdomain"`
	Language  string `yaml:"language"`
	Role      string `yaml:"role"`
	PubLevel  string `yaml:"pub_level"` // top/known/regular/preprint/thesis/undecided

	// Detailed mode fields (optional)
	ResearchType       string   `yaml:"research_type,omitempty"`
	ConcurrentProjects string   `yaml:"concurrent_projects,omitempty"`
	Weaknesses         []string `yaml:"weaknesses,omitempty"`
	LaTeXLevel         string   `yaml:"latex_level,omitempty"`
	Collaboration      string   `yaml:"collaboration,omitempty"`
	FeedbackStyle      string   `yaml:"feedback_style,omitempty"`
	SpecialNeeds       string   `yaml:"special_needs,omitempty"`
}

const profileFileName = "profile.yaml"

// ProfilePath returns the full path to the profile.yaml file.
func ProfilePath(workingDir string) string {
	return filepath.Join(workingDir, config.DefaultDataDir(), profileFileName)
}

// LoadProfile reads the user profile from .openscholar/profile.yaml.
// Returns nil (no error) if the file does not exist.
func LoadProfile(workingDir string) (*UserProfile, error) {
	data, err := os.ReadFile(ProfilePath(workingDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profile: %w", err)
	}

	var profile UserProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	return &profile, nil
}

// SaveProfile writes the user profile to .openscholar/profile.yaml.
func SaveProfile(workingDir string, profile *UserProfile) error {
	dir := filepath.Join(workingDir, config.DefaultDataDir())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	return fileop.WriteFileAtomic(ProfilePath(workingDir), data, 0644)
}

// FormatWeaknesses returns weaknesses as a comma-separated string.
func (p *UserProfile) FormatWeaknesses() string {
	if len(p.Weaknesses) == 0 {
		return ""
	}
	return strings.Join(p.Weaknesses, ", ")
}

// PubLevelDisplay returns a human-readable publication level description.
func (p *UserProfile) PubLevelDisplay() string {
	switch p.PubLevel {
	case "top":
		return "Top-tier venues"
	case "known":
		return "Well-known venues"
	case "regular":
		return "Regular journals"
	case "preprint":
		return "Preprint (arXiv, etc.)"
	case "thesis":
		return "Thesis/Dissertation"
	case "undecided":
		return "No specific target"
	default:
		return p.PubLevel
	}
}
