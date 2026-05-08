package validate

import (
	"sort"

	"github.com/Nahasma/openscholar-public/internal/docx"
)

// DiagLevel indicates the severity of a diagnostic.
type DiagLevel int

const (
	DiagError   DiagLevel = iota
	DiagWarning
	DiagInfo
)

// String returns the level name.
func (d DiagLevel) String() string {
	switch d {
	case DiagError:
		return "error"
	case DiagWarning:
		return "warning"
	case DiagInfo:
		return "info"
	default:
		return "unknown"
	}
}

// Diagnostic is a single validation finding.
type Diagnostic struct {
	Level      DiagLevel
	RuleID     string
	Location   docx.StableAnchor
	Message    string
	Suggestion string
	AutoFix    bool
}

// Rule is the interface all validation rules must implement.
type Rule interface {
	ID() string
	Name() string
	Level() DiagLevel
	Applicable(docType string) bool
	Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic
}

// docTypeAware is an optional interface that rules can implement when they
// need the document type inside their Check method. Validate will call
// SetDocType before Check when the rule implements this interface.
type docTypeAware interface {
	SetDocType(docType string)
}

// Validator runs a set of rules against a document.
type Validator struct {
	rules []Rule
}

// NewValidator creates a validator with the given rules.
func NewValidator(rules ...Rule) *Validator {
	return &Validator{rules: rules}
}

// Validate runs all applicable rules and returns diagnostics sorted by severity.
func (v *Validator) Validate(pkg *docx.Package, nodes []docx.ViewNode, docType string) []Diagnostic {
	var all []Diagnostic
	for _, r := range v.rules {
		if r.Applicable(docType) {
			// Inject docType if the rule needs it.
			if dt, ok := r.(docTypeAware); ok {
				dt.SetDocType(docType)
			}
			all = append(all, r.Check(pkg, nodes)...)
		}
	}
	// Sort: errors first, then warnings, then info
	sort.Slice(all, func(i, j int) bool {
		return all[i].Level < all[j].Level
	})
	return all
}

// DefaultRules returns all P0 rules (format + content).
func DefaultRules() []Rule {
	return append(FormatRules(), ContentRules()...)
}

// AllRules returns P0 + P1 rules.
func AllRules() []Rule {
	rules := DefaultRules()
	rules = append(rules, DomainRules()...)
	return rules
}

// DomainRules is implemented in rules_domain.go.
