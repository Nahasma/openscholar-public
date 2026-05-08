package outputstyle

// Style represents a loaded output style definition.
type Style struct {
	Name                   string // from frontmatter or filename
	Description            string // from frontmatter
	KeepCodingInstructions bool   // preserve coding style if true
	Content                string // the prompt content (body after frontmatter)
	Source                 string // "project" or "user"
}
