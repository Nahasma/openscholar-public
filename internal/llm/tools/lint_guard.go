package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// LintResult holds the outcome of a lint check on a file.
type LintResult struct {
	// Passed is true when the file has no lint errors.
	Passed bool
	// Output contains the raw linter output (warnings + errors).
	Output string
}

// LintGuard performs post-write lint checks on edited files.
// It supports .tex files (via chktex or pdflatex) and .go files (via go build).
type LintGuard struct {
	latexEnabled bool
	goEnabled    bool
}

// NewLintGuard creates a LintGuard with the given feature flags.
func NewLintGuard(latexEnabled, goEnabled bool) *LintGuard {
	return &LintGuard{
		latexEnabled: latexEnabled,
		goEnabled:    goEnabled,
	}
}

// Check runs the appropriate linter for the given file path.
// Returns nil when the file extension is not handled, or when the relevant
// feature flag is disabled.
func (g *LintGuard) Check(filePath string) *LintResult {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".tex":
		if !g.latexEnabled {
			return nil
		}
		return g.checkLatex(filePath)
	case ".go":
		if !g.goEnabled {
			return nil
		}
		return g.checkGo(filePath)
	default:
		return nil
	}
}

// checkLatex lints a .tex file.
// It first tries chktex -q; if chktex is not available it falls back to
// pdflatex -draftmode (which catches compilation errors without producing a PDF).
func (g *LintGuard) checkLatex(filePath string) *LintResult {
	dir := filepath.Dir(filePath)

	// Try chktex first (fast, syntax-focused)
	if path, err := exec.LookPath("chktex"); err == nil && path != "" {
		cmd := exec.Command("chktex", "-q", filePath)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		output := strings.TrimSpace(out.String())
		if err != nil {
			// chktex exits non-zero when warnings/errors are found
			return &LintResult{Passed: false, Output: fmt.Sprintf("chktex: %s", output)}
		}
		return &LintResult{Passed: true, Output: output}
	}

	// Fallback: pdflatex -draftmode (only checks for compilation errors)
	if path, err := exec.LookPath("pdflatex"); err == nil && path != "" {
		cmd := exec.Command("pdflatex", "-draftmode", "-interaction=nonstopmode", filePath)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		output := strings.TrimSpace(out.String())
		if err != nil {
			return &LintResult{Passed: false, Output: fmt.Sprintf("pdflatex: %s", output)}
		}
		return &LintResult{Passed: true, Output: output}
	}

	// Neither tool available — skip silently
	return nil
}

// checkGo lints a .go file by running `go build ./...` in the file's directory.
func (g *LintGuard) checkGo(filePath string) *LintResult {
	dir := filepath.Dir(filePath)

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := strings.TrimSpace(out.String())
	if err != nil {
		return &LintResult{Passed: false, Output: fmt.Sprintf("go build: %s", output)}
	}
	return &LintResult{Passed: true, Output: output}
}
