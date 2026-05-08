package modules

// NewCrossCompareModule returns the prompt module for structured paper comparison.
// Priority 73: loads after cross_reading (72).
func NewCrossCompareModule() BaseModule {
	return NewBaseModule("cross_compare", crossComparePrompt, 73)
}

const crossComparePrompt = `# Structured Paper Comparison

When asked to compare multiple papers, use the KBSearch tool to retrieve relevant content, then produce:

## Step 1: Per-Paper Extraction
For each paper, extract:
- **Problem**: What problem does it solve?
- **Method**: Core approach / algorithm / architecture
- **Data**: Datasets and benchmarks used
- **Results**: Key quantitative results (tables/figures)
- **Limitations**: Acknowledged weaknesses

## Step 2: Comparison Table
Generate a LaTeX comparison table:

\begin{table}[ht]
\centering
\caption{Comparison of approaches}
\begin{tabular}{l|ccc}
\toprule
\textbf{Dimension} & \textbf{Paper A} & \textbf{Paper B} & \textbf{Paper C} \\
\midrule
Problem     & ... & ... & ... \\
Method      & ... & ... & ... \\
Dataset     & ... & ... & ... \\
Key Result  & ... & ... & ... \\
Limitation  & ... & ... & ... \\
\bottomrule
\end{tabular}
\end{table}

## Step 3: Synthesis
Write a 1-2 paragraph synthesis covering:
- Consensus across papers
- Key divergences
- Complementary contributions
- Remaining gaps in the literature
`
