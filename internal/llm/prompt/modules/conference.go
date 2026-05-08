package modules

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/template"
)

// NewConferenceModule returns a conference-specific prompt module based on the detected template.
// Returns nil if cfg is nil. The conference module is dynamic because it depends on the
// working directory's detected template (may differ between sessions).
func NewConferenceModule(cfg *template.TemplateConfig) *DynamicBaseModule {
	if cfg == nil {
		return nil
	}
	content := buildConferencePrompt(cfg)
	m := NewDynamicBaseModule("conference", content, 40)
	return &m
}

func buildConferencePrompt(cfg *template.TemplateConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Conference: %s %d\n", cfg.Conference, cfg.Year))

	if cfg.PageLimit > 0 {
		sb.WriteString(fmt.Sprintf(`
## Page limit
- Body text must not exceed %d pages
- References and appendix typically do not count toward the page limit
- After major edits, remind the user to check the page count
`, cfg.PageLimit))
	}

	if cfg.Anonymous {
		sb.WriteString(`
## Anonymization rules (double-blind)
- No author names or affiliations in the manuscript
- \author{} must use "Anonymous" or the template's default anonymous marker
- No self-identifying citations: use "Prior work [1] shows..." instead of "Our previous work [1]..."
- Remove or guard acknowledgements with \ifdefined\isaccepted
`)
	}

	if cfg.Compiler != "" {
		sb.WriteString(fmt.Sprintf(`
## Compiler
- Use %s for compilation
`, cfg.Compiler))
	}

	sb.WriteString(`
## Figure specifications
- Single-column figure width: \columnwidth; double-column: \textwidth
- Resolution >= 300 DPI
- Prefer vector formats (PDF/EPS) over raster (PNG/JPG) for charts and diagrams
`)

	return sb.String()
}
