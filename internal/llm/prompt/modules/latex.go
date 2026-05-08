package modules

// NewLatexModule returns the LaTeX conventions module.
func NewLatexModule() BaseModule {
	return NewBaseModule("latex", latexPrompt, 10)
}

const latexPrompt = `# LaTeX conventions

## Document structure
- Use \input{} to split sections; the main file should only contain structure
- Each section in its own .tex file for parallel editing
- Images in a figures/ directory

## Cross-references
- Semantic label prefixes: fig:, tab:, sec:, eq:, alg:
- Use \cref{} (cleveref) or \ref{} for references
- Every \label must have a corresponding \ref

## Math
- Inline formulas: $...$
- Display formulas: equation environment (numbered) or \[...\] (unnumbered)
- Multi-line: align environment, align on =
- Bold vectors: \mathbf{}, uppercase for matrices
- In terminal answers with many formulas, avoid placing complex LaTeX inside Markdown tables; use separate display equations plus short labels so the TUI can preserve readable source.`
