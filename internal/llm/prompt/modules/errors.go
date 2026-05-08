package modules

// NewErrorsModule returns the LaTeX compilation troubleshooting module.
func NewErrorsModule() BaseModule {
	return NewBaseModule("errors", errorsPrompt, 25)
}

const errorsPrompt = `# LaTeX compilation troubleshooting

When the user reports a compilation error or you encounter one after running latexmk/pdflatex/xelatex, follow this workflow.

## fix-compile-verify loop
1. Locate: read the .log file, find the first error (first error wins)
2. Fix: apply the corresponding fix
3. Verify: recompile and check
4. Repeat: if more errors remain, go back to step 1

## 8 common error patterns

### 1. Undefined control sequence
- Cause: typo or missing package
- Fix: check spelling → add \usepackage{}

### 2. Missing $ inserted
- Cause: math symbols (_, ^, \alpha) used in text mode
- Fix: wrap in $...$ or use \textsubscript{}

### 3. Environment undefined
- Cause: undefined environment name
- Fix: check spelling → add the required package

### 4. Too many unprocessed floats
- Cause: consecutive float environments piling up
- Fix: insert \clearpage at appropriate points, or use [H] (float package)

### 5. File not found
- Cause: wrong path in \input{} or \includegraphics{}
- Fix: use Glob to verify the actual path → correct it

### 6. Citation undefined
- Cause: \cite{key} not found in .bib
- Fix: use Grep to search .bib for the correct key → fix the citation

### 7. Overfull/Underfull hbox
- Cause: line width overflow or underflow
- Fix: overfull — check long formulas/URLs/tables for line breaks; underfull — usually ignorable (warning level)

### 8. Package clash / Option clash
- Cause: same package loaded twice with different options
- Fix: use \PassOptionsToPackage{option}{package} before \documentclass, or unify load options`
