package modules

// NewTablesModule returns the table generation best practices module.
func NewTablesModule() BaseModule {
	return NewBaseModule("tables", tablesPrompt, 20)
}

const tablesPrompt = `# Table generation best practices

## booktabs (mandatory)
- Always use booktabs: \toprule, \midrule, \bottomrule
- Never use \hline or vertical lines |
- Separate header from data with \midrule
- Group sub-headers with \cmidrule(lr){start-end}

## Numeric alignment (siunitx)
- Numeric columns: S[table-format=2.1]
- Percentages: S[table-format=2.1] with \si{\percent}
- Non-numeric cells: wrap in {text} to exempt from S-column parsing

## Highlighting best values
- Best value: \textbf{95.3}
- Second best: \underline{94.1}
- In S columns: {\textbf{95.3}}

## Caption rules
- \caption must appear before \begin{tabular} (above the table)
- End caption text with a period

## Sizing
- Wide tables: \resizebox{\columnwidth}{!}{...} or \adjustbox{max width=\textwidth}{...}
- Avoid \small/\footnotesize for scaling — prefer adjusting column widths
- Very wide tables: consider splitting or landscape environment`
