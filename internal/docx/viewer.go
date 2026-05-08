package docx

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/beevik/etree"
)

// View converts a Package into a ViewNode tree by traversing
// word/document.xml (and optionally headers/footers).
func View(pkg *Package) ([]ViewNode, error) {
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("docx: view: %w", err)
	}

	var nodes []ViewNode

	err = Traverse(doc, func(elem *etree.Element, index int) error {
		node, ok := elementToNode(elem, "word/document.xml")
		if ok {
			nodes = append(nodes, node)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// elementToNode converts a single top-level body element into a ViewNode.
// Returns (node, true) when the element should be included, (zero, false) to skip.
func elementToNode(elem *etree.Element, part string) (ViewNode, bool) {
	anchor := StableAnchor{Part: part}

	// Attempt to read w14:paraId
	paraID := ""
	for _, attr := range elem.Attr {
		if attr.Key == "paraId" {
			paraID = attr.Value
			break
		}
	}
	anchor.ParaID = paraID

	switch elem.Tag {
	case "p":
		return paragraphNode(elem, anchor), true

	case "tbl":
		return tableNode(elem, anchor), true

	case "sdt":
		return sdtNode(elem, anchor), true

	case "sectPr":
		// Section properties – skip
		return ViewNode{}, false

	case "bookmarkStart", "bookmarkEnd":
		// Bookmarks are recorded on paragraphs, not standalone nodes
		return ViewNode{}, false

	default:
		return ViewNode{}, false
	}
}

// paragraphNode converts a w:p element into a ViewNode.
func paragraphNode(elem *etree.Element, anchor StableAnchor) ViewNode {
	node := ViewNode{
		Type:   NodeParagraph,
		Anchor: anchor,
	}

	// Check for w:pPr / w:pStyle
	ppr := elem.FindElement("pPr")
	if ppr != nil {
		styleElem := ppr.FindElement("pStyle")
		if styleElem != nil {
			styleVal := styleVal(styleElem)
			if level, ok := headingLevel(styleVal); ok {
				node.Type = NodeHeading
				node.Level = level
			}
		}

		// Check for list (w:numPr)
		numPr := ppr.FindElement("numPr")
		if numPr != nil && node.Type == NodeParagraph {
			node.Type = NodeListItem
			ilvl := numPr.FindElement("ilvl")
			if ilvl != nil {
				lvlVal := ilvl.SelectAttrValue("val", "0")
				n := 0
				fmt.Sscanf(lvlVal, "%d", &n)
				node.Level = n + 1
			} else {
				node.Level = 1
			}
		}
	}

	// Check for inline drawing/image
	hasImage := false
	for _, child := range elem.ChildElements() {
		if child.Tag == "r" {
			for _, rc := range child.ChildElements() {
				if rc.Tag == "drawing" || rc.Tag == "pict" {
					hasImage = true
					rID := extractImageRID(rc)
					imgNode := ViewNode{
						Type:   NodeImage,
						Anchor: anchor,
						Metadata: map[string]string{
							"rId": rID,
						},
					}
					node.Children = append(node.Children, imgNode)
				}
			}
		}
	}
	_ = hasImage

	text, styles := RenderParagraph(elem)
	node.Content = text
	node.Style = styles

	// Collect bookmark names from sibling bookmarkStart elements
	for _, child := range elem.ChildElements() {
		if child.Tag == "bookmarkStart" {
			name := child.SelectAttrValue("name", "")
			if name != "" {
				anchor.Bookmark = name
				node.Anchor = anchor
			}
		}
	}

	return node
}

// styleVal extracts the "val" attribute from a w:pStyle element.
func styleVal(styleElem *etree.Element) string {
	val := styleElem.SelectAttrValue("val", "")
	if val == "" {
		for _, attr := range styleElem.Attr {
			if attr.Key == "val" {
				val = attr.Value
				break
			}
		}
	}
	return val
}

// headingLevel returns the heading level (1-9) for a pStyle value, or 0/false.
func headingLevel(styleVal string) (int, bool) {
	// Match "Heading1", "Heading2", ... "heading1", "heading2", "1", "2" etc.
	lower := strings.ToLower(styleVal)

	// Check "heading" prefix
	if strings.HasPrefix(lower, "heading") {
		suffix := lower[len("heading"):]
		// Remove any non-digit separator
		suffix = strings.TrimLeft(suffix, " -_")
		n := 0
		if _, err := fmt.Sscanf(suffix, "%d", &n); err == nil && n >= 1 && n <= 9 {
			return n, true
		}
		// "Heading" without number → level 1
		if suffix == "" {
			return 1, true
		}
	}

	// Check pure digit style names like "1", "2" ... used by some templates
	n := 0
	if _, err := fmt.Sscanf(styleVal, "%d", &n); err == nil && n >= 1 && n <= 9 {
		// Ambiguous — only treat as heading if it's a single digit
		if len(strings.TrimSpace(styleVal)) == 1 {
			return n, true
		}
	}

	return 0, false
}

// tableNode converts a w:tbl element into a ViewNode.
func tableNode(elem *etree.Element, anchor StableAnchor) ViewNode {
	node := ViewNode{
		Type:   NodeTable,
		Anchor: anchor,
	}

	for _, tr := range elem.ChildElements() {
		if tr.Tag != "tr" {
			continue
		}
		rowNode := ViewNode{Type: NodeParagraph} // reuse NodeParagraph as row container
		for _, tc := range tr.ChildElements() {
			if tc.Tag != "tc" {
				continue
			}
			// Concatenate all paragraphs in the cell
			var cellText strings.Builder
			for _, p := range tc.ChildElements() {
				if p.Tag == "p" {
					t, _ := RenderParagraph(p)
					if cellText.Len() > 0 && t != "" {
						cellText.WriteString(" ")
					}
					cellText.WriteString(t)
				}
			}
			cellNode := ViewNode{
				Type:    NodeParagraph,
				Content: cellText.String(),
			}
			rowNode.Children = append(rowNode.Children, cellNode)
		}
		node.Children = append(node.Children, rowNode)
	}

	return node
}

// sdtNode converts a w:sdt element into a ViewNode.
func sdtNode(elem *etree.Element, anchor StableAnchor) ViewNode {
	node := ViewNode{
		Type:     NodeContentControl,
		Anchor:   anchor,
		Metadata: make(map[string]string),
	}

	// Try to find the alias: w:sdt/w:sdtPr/w:alias[@w:val]
	sdtPr := elem.FindElement("sdtPr")
	if sdtPr != nil {
		alias := sdtPr.FindElement("alias")
		if alias != nil {
			val := alias.SelectAttrValue("val", "")
			if val == "" {
				for _, attr := range alias.Attr {
					if attr.Key == "val" {
						val = attr.Value
						break
					}
				}
			}
			node.Metadata["alias"] = val
		}
	}

	// Extract text content from w:sdtContent
	sdtContent := elem.FindElement("sdtContent")
	if sdtContent != nil {
		var sb strings.Builder
		for _, child := range sdtContent.ChildElements() {
			if child.Tag == "p" {
				t, _ := RenderParagraph(child)
				sb.WriteString(t)
			}
		}
		node.Content = sb.String()
	}

	return node
}

// extractImageRID extracts the relationship ID from a w:drawing or w:pict element.
func extractImageRID(elem *etree.Element) string {
	// w:drawing contains wp:inline/a:graphic/a:graphicData/pic:pic/pic:blipFill/a:blip[@r:embed]
	// We search recursively for the embed attribute.
	return findEmbedRID(elem)
}

func findEmbedRID(elem *etree.Element) string {
	for _, attr := range elem.Attr {
		if attr.Key == "embed" {
			return attr.Value
		}
	}
	for _, child := range elem.ChildElements() {
		if rid := findEmbedRID(child); rid != "" {
			return rid
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// RenderMarkdown
// ---------------------------------------------------------------------------

// RenderMarkdown converts a ViewNode tree to a Markdown string.
func RenderMarkdown(nodes []ViewNode) string {
	var sb strings.Builder
	for _, n := range nodes {
		renderMarkdownNode(&sb, n)
	}
	return sb.String()
}

func renderMarkdownNode(sb *strings.Builder, n ViewNode) {
	switch n.Type {
	case NodeHeading:
		level := max(n.Level, 1)
		level = min(level, 6)
		sb.WriteString(strings.Repeat("#", level))
		sb.WriteString(" ")
		sb.WriteString(n.Content)
		sb.WriteString("\n\n")

	case NodeParagraph:
		if n.Content != "" {
			sb.WriteString(n.Content)
			sb.WriteString("\n\n")
		}

	case NodeListItem:
		indent := ""
		if n.Level > 1 {
			indent = strings.Repeat("  ", n.Level-1)
		}
		sb.WriteString(indent)
		sb.WriteString("- ")
		sb.WriteString(n.Content)
		sb.WriteString("\n")

	case NodeTable:
		renderMarkdownTable(sb, n)

	case NodeImage:
		rID := ""
		if n.Metadata != nil {
			rID = n.Metadata["rId"]
		}
		fmt.Fprintf(sb, "![image](%s)\n\n", rID)

	case NodeMath:
		sb.WriteString("$")
		sb.WriteString(n.Content)
		sb.WriteString("$\n\n")

	case NodeFootnote:
		id := ""
		if n.Metadata != nil {
			id = n.Metadata["id"]
		}
		fmt.Fprintf(sb, "[^%s]\n\n", id)

	case NodeContentControl:
		alias := ""
		if n.Metadata != nil {
			alias = n.Metadata["alias"]
		}
		fmt.Fprintf(sb, "[CC: %s] %s\n\n", alias, n.Content)
	}
}

func renderMarkdownTable(sb *strings.Builder, n ViewNode) {
	if len(n.Children) == 0 {
		return
	}

	// Determine column count from first row
	cols := max(len(n.Children[0].Children), 0)
	if cols == 0 {
		return
	}

	// Header row
	row0 := n.Children[0]
	sb.WriteString("|")
	for ci := 0; ci < cols; ci++ {
		cell := ""
		if ci < len(row0.Children) {
			cell = row0.Children[ci].Content
		}
		sb.WriteString(" ")
		sb.WriteString(cell)
		sb.WriteString(" |")
	}
	sb.WriteString("\n")

	// Separator
	sb.WriteString("|")
	for ci := 0; ci < cols; ci++ {
		sb.WriteString(" --- |")
	}
	sb.WriteString("\n")

	// Data rows
	for _, row := range n.Children[1:] {
		sb.WriteString("|")
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(row.Children) {
				cell = row.Children[ci].Content
			}
			sb.WriteString(" ")
			sb.WriteString(cell)
			sb.WriteString(" |")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// ---------------------------------------------------------------------------
// RenderTerminal
// ---------------------------------------------------------------------------

// RenderTerminal converts a ViewNode tree to a terminal-friendly
// plain-text format suitable for the View tool output.
func RenderTerminal(nodes []ViewNode, width int) string {
	var sb strings.Builder
	for _, n := range nodes {
		renderTerminalNode(&sb, n, width)
	}
	return sb.String()
}

func renderTerminalNode(sb *strings.Builder, n ViewNode, width int) {
	switch n.Type {
	case NodeHeading:
		level := max(n.Level, 1)
		prefix := strings.Repeat("=", level) + " "
		suffix := " " + strings.Repeat("=", level)
		line := prefix + n.Content + suffix
		if width > 0 && utf8.RuneCountInString(line) > width {
			line = truncateLine(line, width)
		}
		sb.WriteString(line)
		sb.WriteString("\n\n")

	case NodeParagraph:
		if n.Content != "" {
			line := n.Content
			if width > 0 && utf8.RuneCountInString(line) > width {
				line = wordWrap(line, width)
			}
			sb.WriteString(line)
			sb.WriteString("\n\n")
		}

	case NodeListItem:
		indent := strings.Repeat("  ", n.Level-1)
		line := indent + "• " + n.Content
		if width > 0 && utf8.RuneCountInString(line) > width {
			line = truncateLine(line, width)
		}
		sb.WriteString(line)
		sb.WriteString("\n")

	case NodeTable:
		renderTerminalTable(sb, n, width)

	case NodeImage:
		rID := ""
		if n.Metadata != nil {
			rID = n.Metadata["rId"]
		}
		fmt.Fprintf(sb, "[Image: %s]\n\n", rID)

	case NodeMath:
		sb.WriteString("[Math: ")
		sb.WriteString(n.Content)
		sb.WriteString("]\n\n")

	case NodeContentControl:
		alias := ""
		if n.Metadata != nil {
			alias = n.Metadata["alias"]
		}
		fmt.Fprintf(sb, "[%s]: %s\n\n", alias, n.Content)
	}
}

func renderTerminalTable(sb *strings.Builder, n ViewNode, width int) {
	if len(n.Children) == 0 {
		return
	}

	cols := 0
	for _, row := range n.Children {
		if len(row.Children) > cols {
			cols = len(row.Children)
		}
	}
	if cols == 0 {
		return
	}

	// Compute column widths
	colWidths := make([]int, cols)
	for _, row := range n.Children {
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(row.Children) {
				cell = row.Children[ci].Content
			}
			w := utf8.RuneCountInString(cell)
			if w > colWidths[ci] {
				colWidths[ci] = w
			}
		}
	}

	// Respect total width if set
	if width > 0 {
		totalWidth := cols + 1 // separators
		for _, w := range colWidths {
			totalWidth += w + 2 // padding
		}
		if totalWidth > width {
			// Scale down proportionally
			excess := totalWidth - width
			for i := range colWidths {
				if excess <= 0 {
					break
				}
				reduce := min(colWidths[i]/3, excess)
				if colWidths[i]-reduce < 3 {
					reduce = colWidths[i] - 3
				}
				if reduce > 0 {
					colWidths[i] -= reduce
					excess -= reduce
				}
			}
		}
	}

	separator := buildSeparator(colWidths)

	sb.WriteString(separator)
	sb.WriteString("\n")

	for ri, row := range n.Children {
		sb.WriteString("|")
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(row.Children) {
				cell = row.Children[ci].Content
			}
			if utf8.RuneCountInString(cell) > colWidths[ci] {
				cell = truncateLine(cell, colWidths[ci])
			}
			sb.WriteString(" ")
			sb.WriteString(cell)
			// Pad to column width
			pad := colWidths[ci] - utf8.RuneCountInString(cell)
			sb.WriteString(strings.Repeat(" ", pad))
			sb.WriteString(" |")
		}
		sb.WriteString("\n")

		// Separator after header row
		if ri == 0 {
			sb.WriteString(separator)
			sb.WriteString("\n")
		}
	}
	sb.WriteString(separator)
	sb.WriteString("\n\n")
}

func buildSeparator(colWidths []int) string {
	var sb strings.Builder
	sb.WriteString("+")
	for _, w := range colWidths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	return sb.String()
}

func truncateLine(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func wordWrap(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var sb strings.Builder
	lineLen := 0
	for i, w := range words {
		wLen := utf8.RuneCountInString(w)
		if i == 0 {
			sb.WriteString(w)
			lineLen = wLen
		} else if lineLen+1+wLen > width {
			sb.WriteString("\n")
			sb.WriteString(w)
			lineLen = wLen
		} else {
			sb.WriteString(" ")
			sb.WriteString(w)
			lineLen += 1 + wLen
		}
	}
	return sb.String()
}
