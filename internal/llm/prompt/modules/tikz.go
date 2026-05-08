package modules

// NewTikZModule returns the prompt module for TikZ/PGFPlots guidance.
// Priority 75: loads after rebuttal (74).
func NewTikZModule() BaseModule {
	return NewBaseModule("tikz", tikzPrompt, 75)
}

const tikzPrompt = `# TikZ / PGFPlots Figure Generation

When generating technical diagrams or plots for the paper:

## When to Use TikZ vs Other Tools
- **TikZ**: Architecture diagrams, flowcharts, neural network diagrams, process flows
- **PGFPlots**: Line charts, bar charts, scatter plots, histograms from data
- **DiagramGen (D2)**: Quick system diagrams, sequence diagrams
- **ImageGen**: Conceptual illustrations, non-technical images

## TikZ Best Practices
1. Always use \usetikzlibrary{} for needed libraries (arrows.meta, positioning, calc, fit)
2. Use relative positioning (\node[right=of A]) over absolute coordinates
3. Define styles at the top: \tikzset{block/.style={...}}
4. Use named nodes for clarity: \node (encoder) {...}
5. Keep diagrams simple — avoid over-decoration

## PGFPlots Best Practices
1. Use \pgfplotsset{compat=1.18} for latest features
2. Include axis labels and units: xlabel={Epoch}, ylabel={Accuracy (\%)}
3. Use legend entries with \addlegendentry{}
4. Set appropriate axis limits and tick marks

## Verification
After generating TikZ code, verify by compiling:
` + "```bash\npdflatex -interaction=nonstopmode tikz_figure.tex\n```" + `
Check for errors and fix before including in the paper.
`
