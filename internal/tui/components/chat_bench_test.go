package components

import (
	"fmt"
	"strings"
	"testing"
)

// generateLongMarkdown creates a markdown document of approximately 500 lines.
func generateLongMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Research Paper: Deep Learning Advances\n\n")
	sb.WriteString("## Abstract\n\nThis paper presents a comprehensive overview of recent advances in deep learning.\n\n")

	for section := 1; section <= 10; section++ {
		fmt.Fprintf(&sb, "## Section %d: Topic Analysis\n\n", section)
		fmt.Fprintf(&sb, "This section covers topic %d in detail. We examine the theoretical foundations and practical implications.\n\n", section)

		// Add a list
		sb.WriteString("Key findings:\n\n")
		for item := 1; item <= 5; item++ {
			fmt.Fprintf(&sb, "- Finding %d.%d: Important result with statistical significance p<0.05\n", section, item)
		}
		sb.WriteString("\n")

		// Add a code block
		sb.WriteString("```python\n")
		fmt.Fprintf(&sb, "# Example implementation for section %d\n", section)
		sb.WriteString("import numpy as np\n")
		sb.WriteString("import torch\n")
		sb.WriteString("import torch.nn as nn\n\n")
		fmt.Fprintf(&sb, "class Model%d(nn.Module):\n", section)
		sb.WriteString("    def __init__(self, input_dim, hidden_dim, output_dim):\n")
		sb.WriteString("        super().__init__()\n")
		sb.WriteString("        self.fc1 = nn.Linear(input_dim, hidden_dim)\n")
		sb.WriteString("        self.fc2 = nn.Linear(hidden_dim, output_dim)\n")
		sb.WriteString("        self.relu = nn.ReLU()\n\n")
		sb.WriteString("    def forward(self, x):\n")
		sb.WriteString("        x = self.relu(self.fc1(x))\n")
		sb.WriteString("        return self.fc2(x)\n")
		sb.WriteString("```\n\n")

		// Add a table
		sb.WriteString("| Method | Accuracy | F1 Score | Latency |\n")
		sb.WriteString("|--------|----------|----------|---------|\n")
		for row := 1; row <= 4; row++ {
			fmt.Fprintf(&sb, "| Method-%d | %.2f%% | %.3f | %dms |\n",
				row, 90.0+float64(row)*0.5, 0.900+float64(row)*0.005, 10+row*5)
		}
		sb.WriteString("\n")

		fmt.Fprintf(&sb, "### Subsection %d.1: Detailed Analysis\n\n", section)
		sb.WriteString("The detailed analysis reveals several important patterns in the data. ")
		sb.WriteString("These patterns suggest that the model architecture is well-suited for this task. ")
		sb.WriteString("Further experiments confirm the robustness of the approach.\n\n")
	}

	sb.WriteString("## Conclusion\n\nIn this paper, we have presented extensive experimental results demonstrating the effectiveness of our approach.\n\n")
	sb.WriteString("## References\n\n")
	for ref := 1; ref <= 20; ref++ {
		fmt.Fprintf(&sb, "%d. Author%d et al. (202%d). Title of paper %d. In *Proceedings of Conference*, pp. %d-%d.\n",
			ref, ref, ref%4, ref, ref*10, ref*10+8)
	}

	return sb.String()
}

// BenchmarkMarkdownRender benchmarks Glamour rendering of a long markdown document (~500 lines).
func BenchmarkMarkdownRender(b *testing.B) {
	content := generateLongMarkdown()

	b.ResetTimer()
	for b.Loop() {
		RenderMarkdown(content, 100)
	}
}
