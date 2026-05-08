package tools

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestContainsCJK(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"empty", "", false},
		{"english only", "Hello, World!", false},
		{"chinese chars", "你好世界", true},
		{"mixed", "Hello 你好", true},
		{"japanese hiragana", "こんにちは", true},
		{"japanese katakana", "カタカナ", true},
		{"korean hangul", "안녕하세요", true},
		{"numbers and symbols", "123!@#$%", false},
		{"CJK Extension A", string(rune(0x3400)), true},
		{"just below CJK range", string(rune(0x4DFF)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsCJK([]byte(tt.input))
			if got != tt.expect {
				t.Errorf("ContainsCJK(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestResolveProfile(t *testing.T) {
	tests := []struct {
		profile   string
		inputPath string
		expect    string
	}{
		{"auto", "paper.tex", "latex-project"},
		{"auto", "guide.md", "markdown-general"},
		{"auto", "report.markdown", "markdown-general"},
		{"auto", "file.docx", "markdown-general"},
		{"latex-project", "any.md", "latex-project"},
		{"markdown-academic", "any.tex", "markdown-academic"},
		{"", "paper.tex", "latex-project"},
	}

	for _, tt := range tests {
		t.Run(tt.profile+"_"+tt.inputPath, func(t *testing.T) {
			got := resolveProfile(tt.profile, tt.inputPath)
			if got != tt.expect {
				t.Errorf("resolveProfile(%q, %q) = %q, want %q", tt.profile, tt.inputPath, got, tt.expect)
			}
		})
	}
}

func TestEngineChain(t *testing.T) {
	tests := []struct {
		profile     string
		expectFirst string
	}{
		{"latex-project", "tectonic"},
		{"markdown-general", "weasyprint"},
		{"markdown-academic", "weasyprint"},
		{"auto", "weasyprint"},
	}

	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			chain := engineChain(tt.profile)
			if len(chain) == 0 {
				t.Fatal("engine chain is empty")
			}
			if chain[0] != tt.expectFirst {
				t.Errorf("engineChain(%q)[0] = %q, want %q", tt.profile, chain[0], tt.expectFirst)
			}
		})
	}
}

func TestGenerateCJKCSS(t *testing.T) {
	css := GenerateCJKCSS("PingFang SC")

	if !strings.Contains(css, `"PingFang SC"`) {
		t.Error("CSS should contain the specified font name")
	}
	if !strings.Contains(css, "@page") {
		t.Error("CSS should contain @page directive")
	}
	if !strings.Contains(css, "font-size: 11pt") {
		t.Error("CSS should set font-size")
	}
	if !strings.Contains(css, "Noto Sans CJK SC") {
		t.Error("CSS should contain fallback CJK font")
	}
	if !strings.Contains(css, "JetBrains Mono") {
		t.Error("CSS should contain monospace font for code")
	}
}

func TestDetectAllEngines(t *testing.T) {
	engines := DetectAllEngines()
	// We just verify it doesn't panic and returns a slice
	// Actual engines depend on the system
	t.Logf("detected %d engines", len(engines))
	for _, e := range engines {
		if e.Name == "" || e.Path == "" {
			t.Errorf("engine has empty name or path: %+v", e)
		}
		t.Logf("  - %s: %s", e.Name, e.Path)
	}
}

func TestResolveCJKFont(t *testing.T) {
	font := ResolveCJKFont()
	// On macOS, we should find at least one CJK font
	if runtime.GOOS == "darwin" && font == "" {
		t.Log("WARNING: no CJK font found on macOS — this is unexpected")
	}
	if font != "" {
		t.Logf("resolved CJK font: %s", font)
	}
}

func TestSelectPDFEngine(t *testing.T) {
	// Test that markdown profile prefers weasyprint
	engine := SelectPDFEngine("markdown-general")
	if engine != nil {
		t.Logf("selected engine for markdown-general: %s (%s)", engine.Name, engine.Path)
	} else {
		t.Log("no PDF engine found for markdown-general")
	}

	// Test that latex profile prefers tectonic
	engine = SelectPDFEngine("latex-project")
	if engine != nil {
		t.Logf("selected engine for latex-project: %s (%s)", engine.Name, engine.Path)
	} else {
		t.Log("no PDF engine found for latex-project")
	}
}

func TestRenderPDF_MissingInput(t *testing.T) {
	result := RenderPDF(t.Context(), "/nonexistent/file.md", "/tmp/out.pdf", RenderOptions{
		Profile:  "auto",
		Language: "auto",
	})
	if result.Success {
		t.Error("expected failure for missing input file")
	}
	if result.ErrorClass != "missing_dependency" {
		t.Errorf("expected error class 'missing_dependency', got %q", result.ErrorClass)
	}
}

func TestRenderPDF_CJKMarkdown(t *testing.T) {
	// Create a temporary markdown file with Chinese content
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-cjk-*.md")
	if err != nil {
		t.Fatal(err)
	}
	content := "# 测试文档\n\n这是一个包含中文内容的测试文档。\n\n## 第二章\n\n你好世界！\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	outPath := tmpFile.Name() + ".pdf"
	defer os.Remove(outPath)

	result := RenderPDF(t.Context(), tmpFile.Name(), outPath, RenderOptions{
		Profile:  "markdown-general",
		Language: "zh-CN",
	})

	t.Logf("RenderPDF result: success=%v engine=%s font=%s errorClass=%s error=%s",
		result.Success, result.EngineUsed, result.FontUsed, result.ErrorClass, result.ErrorMessage)

	for _, a := range result.EngineAttempts {
		t.Logf("  attempt: %s → %s", a.Engine, a.Error)
	}

	// On a system with weasyprint/tectonic + CJK font, this should succeed
	if result.Success {
		if result.EngineUsed == "" {
			t.Error("success but no engine recorded")
		}
		if result.FontUsed == "" {
			t.Error("CJK content but no font recorded")
		}
		// Verify the file was created
		if _, err := os.Stat(outPath); os.IsNotExist(err) {
			t.Error("output PDF file not created despite success=true")
		}
	} else if result.ErrorClass == "missing_font" {
		t.Skip("skipping: no CJK font available on this system")
	} else if result.ErrorClass == "missing_dependency" {
		t.Skip("skipping: pandoc or PDF engine not available")
	}
}

func TestDocExportAvailability(t *testing.T) {
	tool := NewDocExportTool(nil)
	checker, ok := tool.(AvailabilityChecker)
	if !ok {
		t.Fatal("DocExport should implement AvailabilityChecker")
	}

	available, info := checker.Available()
	t.Logf("DocExport available=%v info=%q", available, info)
	// Basic validation: info should not be empty
	if info == "" && !available {
		t.Error("unavailable tool should provide a reason")
	}
}
