package template

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/openscholar/openscholar/internal/fileop"
	"gopkg.in/yaml.v3"
)

// TemplatesFS is set by the main package at startup.
var TemplatesFS fs.FS

type TemplateConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Conference  string   `yaml:"conference"`
	Year        int      `yaml:"year"`
	Files       []string `yaml:"files"`
	Format      string   `yaml:"format,omitempty"` // "latex" (default), "markdown"

	// Phase 1 additions
	PageLimit int    `yaml:"page_limit,omitempty"` // body page limit (e.g. NeurIPS 9)
	Anonymous bool   `yaml:"anonymous,omitempty"`  // double-blind review
	Compiler  string `yaml:"compiler,omitempty"`   // pdflatex / xelatex / lualatex
	BibStyle  string `yaml:"bib_style,omitempty"`  // natbib / biblatex
}

type InitPlanFile struct {
	RelativePath string
	TargetPath   string
	Exists       bool
}

type InitPlan struct {
	TemplateName string
	Template     TemplateConfig
	TargetDir    string
	Files        []InitPlanFile
}

type ApplyOptions struct {
	Overwrite bool
}

func (p *InitPlan) HasOverwriteRisk() bool {
	if p == nil {
		return false
	}
	for _, f := range p.Files {
		if f.Exists {
			return true
		}
	}
	return false
}

func List() []string {
	if TemplatesFS == nil {
		return nil
	}
	entries, err := fs.ReadDir(TemplatesFS, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// knownDocClasses maps \usepackage or \documentclass patterns to template names.
var knownDocClasses = map[string]string{
	"neurips_2026":        "neurips2026",
	"neurips_2025":        "neurips2026",
	"icml2026":            "icml2026",
	"cvpr":                "cvpr2026",
	"acl":                 "acl2026",
	"aaai26":              "aaai2026",
	"aaai25":              "aaai2026",
	"iclr2026_conference": "iclr2026",
	"iclr2025_conference": "iclr2026",
	"llncs":               "eccv2026",
	"IEEEtran":            "ieee",
	"acmart":              "kdd2026",
}

var docclassRe = regexp.MustCompile(`\\(?:documentclass|usepackage)(?:\[.*?\])?\{(\w+)\}`)

// Detect scans .tex files in dir for known conference templates.
// Returns the matching TemplateConfig or nil if no match.
func Detect(dir string) *TemplateConfig {
	if TemplatesFS == nil {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tex") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		matches := docclassRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			templateName, ok := knownDocClasses[m[1]]
			if !ok {
				continue
			}
			// Load the matching template config
			cfgData, err := fs.ReadFile(TemplatesFS, filepath.Join(templateName, "template.yaml"))
			if err != nil {
				continue
			}
			var cfg TemplateConfig
			if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
				continue
			}
			return &cfg
		}
	}

	// Fallback: check for markdown-based projects via template.yaml in .openscholar/
	cfgPath := filepath.Join(dir, ".openscholar", "template.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		var cfg TemplateConfig
		if err := yaml.Unmarshal(data, &cfg); err == nil && cfg.Format == "markdown" {
			return &cfg
		}
	}

	return nil
}

func Init(templateName, targetDir string) error {
	plan, err := PlanInit(templateName, targetDir)
	if err != nil {
		return err
	}
	if err := ApplyInitPlan(plan); err != nil {
		return err
	}
	fmt.Printf("Initialized %s template in %s\n", plan.Template.Name, targetDir)
	return nil
}

func PlanInit(templateName, targetDir string) (*InitPlan, error) {
	if TemplatesFS == nil {
		return nil, fmt.Errorf("templates not available")
	}

	templateDir := templateName

	// Check template exists
	configData, err := fs.ReadFile(TemplatesFS, filepath.Join(templateDir, "template.yaml"))
	if err != nil {
		return nil, fmt.Errorf("template %q not found. Run 'openscholar init' to see available templates", templateName)
	}

	var cfg TemplateConfig
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return nil, fmt.Errorf("invalid template config: %w", err)
	}

	plan := &InitPlan{
		TemplateName: templateName,
		Template:     cfg,
		TargetDir:    targetDir,
	}

	err = fs.WalkDir(TemplatesFS, templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || d.Name() == "template.yaml" {
			return nil
		}

		relPath, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(targetDir, relPath)
		_, statErr := os.Stat(destPath)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}

		plan.Files = append(plan.Files, InitPlanFile{
			RelativePath: relPath,
			TargetPath:   destPath,
			Exists:       exists,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize template: %w", err)
	}

	sort.Slice(plan.Files, func(i, j int) bool {
		return plan.Files[i].RelativePath < plan.Files[j].RelativePath
	})
	return plan, nil
}

func ApplyInitPlan(plan *InitPlan) error {
	return ApplyInitPlanWithOptions(plan, ApplyOptions{Overwrite: true})
}

func ApplyInitPlanWithOptions(plan *InitPlan, opts ApplyOptions) error {
	if plan == nil {
		return fmt.Errorf("nil init plan")
	}
	if TemplatesFS == nil {
		return fmt.Errorf("templates not available")
	}
	templateDir := plan.TemplateName
	for _, f := range plan.Files {
		src := filepath.Join(templateDir, f.RelativePath)
		data, err := fs.ReadFile(TemplatesFS, src)
		if err != nil {
			return fmt.Errorf("read template file %s: %w", src, err)
		}
		exists := f.Exists
		if !exists {
			if _, err := os.Stat(f.TargetPath); err == nil {
				exists = true
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("check target file %s: %w", f.RelativePath, err)
			}
		}
		if exists && !opts.Overwrite {
			return fmt.Errorf("refusing to overwrite existing file %s", f.RelativePath)
		}
		if err := os.MkdirAll(filepath.Dir(f.TargetPath), 0o755); err != nil {
			return err
		}
		if err := fileop.WriteFileAtomic(f.TargetPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
