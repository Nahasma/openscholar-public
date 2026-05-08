package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// RenderOptions holds PDF rendering preferences.
type RenderOptions struct {
	Profile  string // "auto", "latex-project", "markdown-general", "markdown-academic"
	Language string // "auto", "zh-CN", "en", etc.
}

// PDFEngine represents a discovered PDF rendering backend.
type PDFEngine struct {
	Name string // "weasyprint", "tectonic", "xelatex", "typst", "pdflatex"
	Path string // executable path
}

// RenderResult contains the outcome of a PDF render attempt.
type RenderResult struct {
	Success           bool
	OutputPath        string
	EngineUsed        string
	FontUsed          string
	EngineAttempts    []EngineAttempt
	Warnings          []string
	ErrorClass        string // "missing_dependency", "missing_font", "latex_runtime_error", etc.
	ErrorMessage      string
	Recoverable       bool
	RecommendedAction string
}

// EngineAttempt records one engine's attempt result.
type EngineAttempt struct {
	Engine string
	Error  string
}

// ContainsCJK reports whether data contains any CJK Unicode characters,
// including CJK Unified Ideographs, Extension A, Katakana, Hiragana, and Hangul.
func ContainsCJK(data []byte) bool {
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r != utf8.RuneError {
			switch {
			case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
				return true
			case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
				return true
			case r >= 0x30A0 && r <= 0x30FF: // Katakana
				return true
			case r >= 0x3040 && r <= 0x309F: // Hiragana
				return true
			case r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
				return true
			}
		}
		data = data[size:]
	}
	return false
}

// ResolveCJKFont detects an available CJK font on the current system.
// It first tries fc-list, then falls back to checking known font paths.
// Returns the font family name, or "" if none is found.
func ResolveCJKFont() string {
	// Try fc-list first
	fcPath, err := exec.LookPath("fc-list")
	if err == nil {
		out, err := exec.Command(fcPath, ":lang=zh", "-f", "%{family}\n").Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(string(out), "\n")
			priority := []string{
				// macOS
				"PingFang SC", "Songti SC", "Heiti SC", "STHeiti",
				// Linux
				"Noto Sans CJK SC", "Noto Serif CJK SC", "Source Han Sans SC", "WenQuanYi Zen Hei",
				// Windows
				"Microsoft YaHei", "SimSun", "SimHei",
			}
			// Build a set of available families for fast lookup
			available := make(map[string]bool, len(lines))
			for _, line := range lines {
				// fc-list may return comma-separated families per line
				for fam := range strings.SplitSeq(line, ",") {
					available[strings.TrimSpace(fam)] = true
				}
			}
			for _, name := range priority {
				if available[name] {
					return name
				}
			}
			// If none of the priority fonts found, return the first non-empty line
			for _, line := range lines {
				name := strings.TrimSpace(strings.Split(line, ",")[0])
				if name != "" {
					return name
				}
			}
		}
	}

	// Fallback: check known font paths
	switch runtime.GOOS {
	case "darwin":
		candidates := []struct {
			path string
			name string
		}{
			{"/System/Library/Fonts/PingFang.ttc", "PingFang SC"},
			{"/System/Library/Fonts/STHeiti Light.ttc", "STHeiti"},
			{"/Library/Fonts/Songti.ttc", "Songti SC"},
		}
		for _, c := range candidates {
			if _, err := os.Stat(c.path); err == nil {
				return c.name
			}
		}
	case "linux":
		knownDirs := []string{
			"/usr/share/fonts/noto-cjk",
			"/usr/share/fonts/noto",
			"/usr/share/fonts/opentype/noto",
		}
		for _, dir := range knownDirs {
			if _, err := os.Stat(dir); err == nil {
				return "Noto Sans CJK SC"
			}
		}
	}

	return ""
}

// SelectPDFEngine returns the highest-priority available PDF engine for the given profile.
// If profile is "auto", the profile is inferred from the inputPath extension.
// Returns nil if no supported engine is found.
func SelectPDFEngine(profile string) *PDFEngine {
	chain := engineChain(profile)
	for _, name := range chain {
		if path, err := exec.LookPath(name); err == nil {
			return &PDFEngine{Name: name, Path: path}
		}
	}
	return nil
}

// engineChain returns the ordered list of engine names to try for a given profile.
func engineChain(profile string) []string {
	switch profile {
	case "latex-project":
		return []string{"tectonic", "xelatex", "pdflatex"}
	case "markdown-general", "markdown-academic":
		return []string{"weasyprint", "typst", "xelatex", "tectonic", "pdflatex"}
	default:
		// "auto" or unknown: use the markdown chain as a sensible default
		return []string{"weasyprint", "typst", "xelatex", "tectonic", "pdflatex"}
	}
}

// resolveProfile determines the rendering profile from the file extension when profile is "auto".
func resolveProfile(profile, inputPath string) string {
	if profile != "auto" && profile != "" {
		return profile
	}
	switch strings.ToLower(filepath.Ext(inputPath)) {
	case ".tex":
		return "latex-project"
	case ".md", ".markdown":
		return "markdown-general"
	default:
		return "markdown-general"
	}
}

// DetectAllEngines returns all available PDF engines on the system.
func DetectAllEngines() []*PDFEngine {
	candidates := []string{"weasyprint", "typst", "xelatex", "tectonic", "pdflatex", "lualatex", "wkhtmltopdf"}
	var engines []*PDFEngine
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			engines = append(engines, &PDFEngine{Name: name, Path: path})
		}
	}
	return engines
}

// GenerateCJKCSS returns a CSS stylesheet suitable for weasyprint with CJK font support.
func GenerateCJKCSS(fontName string) string {
	return fmt.Sprintf(`@page {
    size: A4;
    margin: 2cm;
}

body {
    font-family: "%s", "Noto Sans CJK SC", "PingFang SC", sans-serif;
    font-size: 11pt;
    line-height: 1.6;
}

h1, h2, h3 {
    color: #1a1a2e;
}

code {
    font-family: "JetBrains Mono", "Menlo", "Consolas", monospace;
    background: #f5f5f5;
    padding: 2px 4px;
}

pre {
    background: #f5f5f5;
    padding: 1em;
    border-radius: 4px;
    overflow-x: auto;
}

table {
    border-collapse: collapse;
    width: 100%%;
}

th, td {
    border: 1px solid #ddd;
    padding: 8px;
    text-align: left;
}

th {
    background: #f0f0f0;
}

blockquote {
    border-left: 3px solid #ccc;
    margin-left: 0;
    padding-left: 1em;
    color: #666;
}
`, fontName)
}

// RenderPDF renders the file at inputPath to a PDF at outputPath using the best available engine.
// It detects CJK content, selects fonts, tries engines in priority order, and returns a detailed result.
func RenderPDF(ctx context.Context, inputPath, outputPath string, opts RenderOptions) *RenderResult {
	result := &RenderResult{}

	// Read input file for CJK detection
	data, err := os.ReadFile(inputPath)
	if err != nil {
		result.ErrorClass = "missing_dependency"
		result.ErrorMessage = fmt.Sprintf("cannot read input file: %v", err)
		result.Recoverable = false
		result.RecommendedAction = "Ensure the input file exists and is readable."
		return result
	}

	// Resolve effective profile
	profile := resolveProfile(opts.Profile, inputPath)

	// CJK detection and font resolution
	hasCJK := ContainsCJK(data)
	fontName := ""
	if hasCJK {
		fontName = ResolveCJKFont()
		if fontName == "" {
			result.ErrorClass = "missing_font"
			result.ErrorMessage = "CJK content detected but no CJK font found on the system"
			result.Recoverable = true
			result.RecommendedAction = "Install a CJK font such as Noto Sans CJK SC (Linux: apt install fonts-noto-cjk) or PingFang SC (macOS)."
			return result
		}
	}

	// Ensure pandoc is available
	pandocPath, err := exec.LookPath("pandoc")
	if err != nil {
		result.ErrorClass = "missing_dependency"
		result.ErrorMessage = "pandoc not found in PATH"
		result.Recoverable = true
		result.RecommendedAction = "Install pandoc: brew install pandoc (macOS) or apt install pandoc (Linux)."
		return result
	}

	// Build ordered engine chain
	chain := engineChain(profile)

	for _, engineName := range chain {
		enginePath, err := exec.LookPath(engineName)
		if err != nil {
			// Engine not installed, skip silently
			continue
		}

		attempt, ok := tryEngine(ctx, pandocPath, enginePath, engineName, inputPath, outputPath, fontName, hasCJK)
		result.EngineAttempts = append(result.EngineAttempts, attempt)

		if ok {
			result.Success = true
			result.OutputPath = outputPath
			result.EngineUsed = engineName
			result.FontUsed = fontName
			return result
		}
	}

	// All engines failed
	result.Success = false
	result.ErrorClass = "latex_runtime_error"
	if len(result.EngineAttempts) == 0 {
		result.ErrorClass = "missing_dependency"
		result.ErrorMessage = fmt.Sprintf("no supported PDF engine found for profile %q; tried: %s", profile, strings.Join(chain, ", "))
		result.RecommendedAction = "Install weasyprint (pip install weasyprint) or pandoc with a LaTeX distribution."
	} else {
		msgs := make([]string, 0, len(result.EngineAttempts))
		for _, a := range result.EngineAttempts {
			msgs = append(msgs, fmt.Sprintf("%s: %s", a.Engine, a.Error))
		}
		result.ErrorMessage = "all engines failed:\n" + strings.Join(msgs, "\n")
		result.RecommendedAction = "Check the engine-specific error messages above and ensure all dependencies are properly installed."
	}
	result.Recoverable = true
	return result
}

// tryEngine attempts a single pandoc invocation for the given engine.
// It returns an EngineAttempt and a bool indicating success.
func tryEngine(ctx context.Context, pandocPath, enginePath, engineName, inputPath, outputPath, fontName string, hasCJK bool) (EngineAttempt, bool) {
	var args []string
	var cleanupCSS string

	switch engineName {
	case "weasyprint":
		// Write temporary CSS file
		cssContent := GenerateCJKCSS(fontName)
		tmpCSS, err := os.CreateTemp(os.TempDir(), "openscholar-cjk-*.css")
		if err != nil {
			return EngineAttempt{Engine: engineName, Error: fmt.Sprintf("failed to create temp CSS: %v", err)}, false
		}
		cleanupCSS = tmpCSS.Name()
		if _, err := tmpCSS.WriteString(cssContent); err != nil {
			_ = tmpCSS.Close()
			_ = os.Remove(cleanupCSS)
			return EngineAttempt{Engine: engineName, Error: fmt.Sprintf("failed to write temp CSS: %v", err)}, false
		}
		_ = tmpCSS.Close()

		args = []string{
			inputPath,
			"-t", "html5",
			"--css=" + cleanupCSS,
			"--pdf-engine=weasyprint",
			"-o", outputPath,
		}

	case "typst":
		args = []string{
			inputPath,
			"--pdf-engine=typst",
			"-o", outputPath,
		}
		if fontName != "" {
			args = append(args, "-V", "mainfont="+fontName)
		}

	case "xelatex", "tectonic":
		args = []string{
			inputPath,
			"--pdf-engine=" + engineName,
			"-o", outputPath,
		}
		if fontName != "" {
			args = append(args, "-V", "mainfont="+fontName)
		}
		if hasCJK && fontName != "" {
			args = append(args, "-V", "CJKmainfont="+fontName)
		}

	case "pdflatex":
		args = []string{
			inputPath,
			"--pdf-engine=pdflatex",
			"-o", outputPath,
		}
		if hasCJK {
			// pdflatex has limited CJK support; record a warning in result but still attempt
			_ = enginePath // suppress unused warning
		}

	default:
		args = []string{
			inputPath,
			"--pdf-engine=" + engineName,
			"-o", outputPath,
		}
	}

	if cleanupCSS != "" {
		defer os.Remove(cleanupCSS)
	}

	cmd := exec.CommandContext(ctx, pandocPath, args...)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(out))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return EngineAttempt{Engine: engineName, Error: errMsg}, false
	}

	return EngineAttempt{Engine: engineName}, true
}
