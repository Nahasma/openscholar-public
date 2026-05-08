package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/openscholar/openscholar/cmd"
	"github.com/openscholar/openscholar/internal/skillbank"
	"github.com/openscholar/openscholar/internal/template"
)

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:data/skills
var skillsFS embed.FS

func main() {
	// Inject embedded templates
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load templates:", err)
		os.Exit(1)
	}
	template.TemplatesFS = sub

	// Inject embedded system skills
	skillsSub, err := fs.Sub(skillsFS, "data/skills")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load skills:", err)
		os.Exit(1)
	}
	skillbank.BundledFS = skillsSub

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
