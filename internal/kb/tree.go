package kb

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseTreeJSON parses the PageIndex tree JSON into a PaperTree.
func ParseTreeJSON(raw string) (*PaperTree, error) {
	var tree PaperTree
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		return nil, fmt.Errorf("failed to parse tree JSON: %w", err)
	}
	return &tree, nil
}

// FlattenNodes recursively extracts all nodes from the tree into a flat slice.
func FlattenNodes(paperID string, tree *PaperTree) []NodeSummary {
	var result []NodeSummary
	flattenNodesRecursive(paperID, tree.Structure, &result)
	return result
}

func flattenNodesRecursive(paperID string, nodes []TreeNode, result *[]NodeSummary) {
	for _, node := range nodes {
		*result = append(*result, NodeSummary{
			PaperID:   paperID,
			NodeID:    node.NodeID,
			Title:     node.Title,
			StartPage: node.StartIndex,
			EndPage:   node.EndIndex,
			Summary:   node.Summary,
			Content:   node.Content,
		})
		if len(node.Nodes) > 0 {
			flattenNodesRecursive(paperID, node.Nodes, result)
		}
	}
}

// FormatTreeForLLM formats the tree structure for LLM tree search prompt.
// Outputs a compact JSON with node_id, title, and summary (no raw text).
func FormatTreeForLLM(tree *PaperTree) string {
	type compactNode struct {
		NodeID  string        `json:"node_id"`
		Title   string        `json:"title"`
		Summary string        `json:"summary,omitempty"`
		Nodes   []compactNode `json:"nodes,omitempty"`
	}

	var compactify func(nodes []TreeNode) []compactNode
	compactify = func(nodes []TreeNode) []compactNode {
		if len(nodes) == 0 {
			return nil
		}
		result := make([]compactNode, len(nodes))
		for i, n := range nodes {
			result[i] = compactNode{
				NodeID:  n.NodeID,
				Title:   n.Title,
				Summary: n.Summary,
				Nodes:   compactify(n.Nodes),
			}
		}
		return result
	}

	compact := compactify(tree.Structure)
	data, _ := json.MarshalIndent(compact, "", "  ")
	return string(data)
}

// FindNodes locates nodes by IDs in the tree, returning their page ranges.
func FindNodes(tree *PaperTree, nodeIDs []string) []TreeNode {
	idSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}

	var found []TreeNode
	findNodesRecursive(tree.Structure, idSet, &found)
	return found
}

func findNodesRecursive(nodes []TreeNode, idSet map[string]bool, found *[]TreeNode) {
	for _, node := range nodes {
		if idSet[node.NodeID] {
			*found = append(*found, node)
		}
		if len(node.Nodes) > 0 {
			findNodesRecursive(node.Nodes, idSet, found)
		}
	}
}

// FormatTreeAsText formats the tree as an indented text outline.
func FormatTreeAsText(tree *PaperTree) string {
	var sb strings.Builder
	if tree.DocDescription != "" {
		sb.WriteString(tree.DocDescription + "\n\n")
	}
	formatNodesAsText(tree.Structure, 0, &sb)
	return sb.String()
}

func formatNodesAsText(nodes []TreeNode, depth int, sb *strings.Builder) {
	indent := strings.Repeat("  ", depth)
	for _, node := range nodes {
		fmt.Fprintf(sb, "%s[%s] %s (pp. %d-%d)\n", indent, node.NodeID, node.Title, node.StartIndex, node.EndIndex)
		if node.Summary != "" {
			fmt.Fprintf(sb, "%s  → %s\n", indent, node.Summary)
		}
		if len(node.Nodes) > 0 {
			formatNodesAsText(node.Nodes, depth+1, sb)
		}
	}
}
