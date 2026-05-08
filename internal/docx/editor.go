package docx

import (
	"fmt"
	"strings"

	"github.com/beevik/etree"
)

// EditOpType classifies the kind of edit operation.
type EditOpType int

const (
	OpReplaceText     EditOpType = iota // Replace paragraph text
	OpInsertAfter                       // Insert new paragraph after target
	OpInsertBefore                      // Insert new paragraph before target
	OpDeleteParagraph                   // Delete the target paragraph
	OpReplaceSection                    // Replace entire section (heading to next same-level heading)
)

// EditOperation describes a single edit to apply.
type EditOperation struct {
	Type    EditOpType
	Target  StableAnchor      // Which paragraph/section to operate on
	Content string            // New text content (for replace/insert)
	Style   map[string]string // Optional style overrides (e.g. "bold": "true")
}

// EditResult reports the outcome of Apply.
type EditResult struct {
	Applied  int      // Number of operations successfully applied
	Skipped  int      // Number skipped (e.g. protected zones)
	Warnings []string // Human-readable warnings
}

// Apply executes a list of edit operations on the Package.
// It modifies the word/document.xml part in-memory and writes it back via SetXML.
// Operations targeting a protected zone (matching ProtectedZones by anchor) are skipped.
func Apply(pkg *Package, ops []EditOperation, protectedZones ...ProtectedZone) (*EditResult, error) {
	if len(ops) == 0 {
		return &EditResult{}, nil
	}

	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("docx: apply: %w", err)
	}

	body := getBody(doc)
	if body == nil {
		return nil, fmt.Errorf("docx: apply: w:body not found in document.xml")
	}

	result := &EditResult{}

	for i, op := range ops {
		// Protected zone check: skip operations targeting a protected region.
		if isProtected(op.Target, protectedZones) {
			result.Skipped++
			result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d]: target is in a protected zone, skipped", i))
			continue
		}

		switch op.Type {
		case OpReplaceText:
			para := findParagraph(body, op.Target)
			if para == nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] ReplaceText: target not found %+v", i, op.Target))
				continue
			}
			replaceParaText(para, op.Content, op.Style)
			result.Applied++

		case OpInsertAfter:
			target := findParagraph(body, op.Target)
			if target == nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] InsertAfter: target not found %+v", i, op.Target))
				continue
			}
			newPara := createParagraph(op.Content, op.Style)
			insertAfterElement(body, target, newPara)
			result.Applied++

		case OpInsertBefore:
			target := findParagraph(body, op.Target)
			if target == nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] InsertBefore: target not found %+v", i, op.Target))
				continue
			}
			newPara := createParagraph(op.Content, op.Style)
			insertBeforeElement(body, target, newPara)
			result.Applied++

		case OpDeleteParagraph:
			target := findParagraph(body, op.Target)
			if target == nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] DeleteParagraph: target not found %+v", i, op.Target))
				continue
			}
			body.RemoveChild(target)
			result.Applied++

		case OpReplaceSection:
			headingPara := findParagraph(body, op.Target)
			if headingPara == nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] ReplaceSection: target heading not found %+v", i, op.Target))
				continue
			}
			if err := replaceSection(body, headingPara, op.Content, op.Style); err != nil {
				result.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d] ReplaceSection: %v", i, err))
				continue
			}
			result.Applied++

		default:
			result.Skipped++
			result.Warnings = append(result.Warnings, fmt.Sprintf("op[%d]: unknown op type %d", i, op.Type))
		}
	}

	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		return nil, fmt.Errorf("docx: apply: write back: %w", err)
	}

	return result, nil
}

// getBody returns the w:body element from the document root.
func getBody(doc *etree.Document) *etree.Element {
	root := doc.Root()
	if root == nil {
		return nil
	}
	for _, child := range root.ChildElements() {
		if child.Tag == "body" {
			return child
		}
	}
	return nil
}

// findParagraph locates a paragraph element in the body that matches the anchor.
// Priority order: ParaID → Bookmark → Offset.
func findParagraph(body *etree.Element, target StableAnchor) *etree.Element {
	children := body.ChildElements()

	// 1. ParaID match
	if target.ParaID != "" {
		for _, child := range children {
			if child.Tag != "p" {
				continue
			}
			for _, attr := range child.Attr {
				if attr.Key == "paraId" && attr.Value == target.ParaID {
					return child
				}
			}
		}
	}

	// 2. Bookmark match — look for bookmarkStart name inside the paragraph
	if target.Bookmark != "" {
		for _, child := range children {
			if child.Tag != "p" {
				continue
			}
			for _, inner := range child.ChildElements() {
				if inner.Tag == "bookmarkStart" {
					name := inner.SelectAttrValue("name", "")
					if name == "" {
						for _, attr := range inner.Attr {
							if attr.Key == "name" {
								name = attr.Value
								break
							}
						}
					}
					if name == target.Bookmark {
						return child
					}
				}
			}
		}
	}

	// 3. Offset match — 0-based index among paragraph children only
	if target.ParaID == "" && target.Bookmark == "" {
		paraIndex := 0
		for _, child := range children {
			if child.Tag != "p" {
				continue
			}
			if paraIndex == target.Offset {
				return child
			}
			paraIndex++
		}
	}

	return nil
}

// replaceParaText clears all runs from the paragraph and inserts a single new run.
func replaceParaText(para *etree.Element, text string, styles map[string]string) {
	// Remove all w:r children, keep w:pPr
	toRemove := make([]*etree.Element, 0)
	for _, child := range para.ChildElements() {
		if child.Tag == "r" || child.Tag == "bookmarkStart" || child.Tag == "bookmarkEnd" {
			// Keep bookmarks as they may be needed for anchoring; remove runs only.
			if child.Tag == "r" {
				toRemove = append(toRemove, child)
			}
		}
	}
	for _, elem := range toRemove {
		para.RemoveChild(elem)
	}

	// Create and append new run
	run := buildRun(text, styles)
	para.AddChild(run)
}

// createParagraph creates a new w:p element with the given text and optional styles.
func createParagraph(text string, styles map[string]string) *etree.Element {
	para := etree.NewElement("w:p")
	run := buildRun(text, styles)
	para.AddChild(run)
	return para
}

// buildRun creates a w:r element with text and optional run properties.
func buildRun(text string, styles map[string]string) *etree.Element {
	run := etree.NewElement("w:r")

	// Apply run properties if any style is requested
	if len(styles) > 0 {
		rpr := etree.NewElement("w:rPr")
		if isTruthy(styles["bold"]) {
			rpr.AddChild(etree.NewElement("w:b"))
		}
		if isTruthy(styles["italic"]) {
			rpr.AddChild(etree.NewElement("w:i"))
		}
		if isTruthy(styles["underline"]) {
			u := etree.NewElement("w:u")
			u.CreateAttr("w:val", "single")
			rpr.AddChild(u)
		}
		if isTruthy(styles["strike"]) {
			rpr.AddChild(etree.NewElement("w:strike"))
		}
		if font, ok := styles["font"]; ok && font != "" {
			rf := etree.NewElement("w:rFonts")
			rf.CreateAttr("w:ascii", font)
			rf.CreateAttr("w:hAnsi", font)
			rpr.AddChild(rf)
		}
		if sz, ok := styles["size"]; ok && sz != "" {
			szElem := etree.NewElement("w:sz")
			szElem.CreateAttr("w:val", sz)
			rpr.AddChild(szElem)
		}
		if len(rpr.ChildElements()) > 0 {
			run.AddChild(rpr)
		}
	}

	t := etree.NewElement("w:t")
	// Preserve leading/trailing spaces with xml:space="preserve"
	if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
		t.CreateAttr("xml:space", "preserve")
	}
	t.SetText(text)
	run.AddChild(t)

	return run
}

// isTruthy returns true for "true", "1", "yes" (case-insensitive).
func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// insertAfterElement inserts newElem right after target in parent's children.
func insertAfterElement(parent, target, newElem *etree.Element) {
	children := parent.ChildElements()
	for i, child := range children {
		if child == target {
			// etree uses token-based child list; use AddChildAt on the token index.
			// We need the token index (which may differ from element index).
			tokenIdx := findTokenIndex(parent, target)
			if tokenIdx >= 0 {
				parent.InsertChildAt(tokenIdx+1, newElem)
			} else {
				// Fallback: append after last child
				if i == len(children)-1 {
					parent.AddChild(newElem)
				} else {
					// Insert before the next sibling element
					parent.InsertChildAt(findTokenIndex(parent, children[i+1]), newElem)
				}
			}
			return
		}
	}
	// target not found — append
	parent.AddChild(newElem)
}

// insertBeforeElement inserts newElem right before target in parent's children.
func insertBeforeElement(parent, target, newElem *etree.Element) {
	tokenIdx := findTokenIndex(parent, target)
	if tokenIdx >= 0 {
		parent.InsertChildAt(tokenIdx, newElem)
	} else {
		// Fallback: prepend
		children := parent.ChildElements()
		if len(children) > 0 {
			parent.InsertChildAt(0, newElem)
		} else {
			parent.AddChild(newElem)
		}
	}
}

// findTokenIndex returns the token-level index of elem within parent's child list.
// etree stores child tokens (elements, chardata, comments) in a flat slice;
// this function locates the element's position within that slice.
func findTokenIndex(parent, elem *etree.Element) int {
	for i, token := range parent.Child {
		if e, ok := token.(*etree.Element); ok && e == elem {
			return i
		}
	}
	return -1
}

// replaceSection replaces all paragraphs between headingPara and the next same-or-higher
// level heading (exclusive) with a single new paragraph.
func replaceSection(body, headingPara *etree.Element, content string, styles map[string]string) error {
	// Determine the heading level of the target paragraph
	level := getParagraphHeadingLevel(headingPara)
	if level == 0 {
		return fmt.Errorf("target paragraph is not a heading")
	}

	children := body.ChildElements()

	// Find start index (the heading itself)
	startIdx := -1
	for i, child := range children {
		if child == headingPara {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return fmt.Errorf("heading paragraph not found in body")
	}

	// Find end index: next heading of same or higher level (lower number), or end of body
	endIdx := len(children) // exclusive
	for i := startIdx + 1; i < len(children); i++ {
		child := children[i]
		if child.Tag != "p" {
			continue
		}
		l := getParagraphHeadingLevel(child)
		if l > 0 && l <= level {
			endIdx = i
			break
		}
	}

	// Collect elements to delete: everything between heading (exclusive) and endIdx (exclusive)
	toDelete := make([]*etree.Element, 0, endIdx-startIdx-1)
	for i := startIdx + 1; i < endIdx; i++ {
		toDelete = append(toDelete, children[i])
	}
	for _, elem := range toDelete {
		body.RemoveChild(elem)
	}

	// Insert the new paragraph after the heading
	newPara := createParagraph(content, styles)
	insertAfterElement(body, headingPara, newPara)

	return nil
}

// getParagraphHeadingLevel returns the heading level (1-9) of a w:p element,
// or 0 if it's not a heading.
func getParagraphHeadingLevel(para *etree.Element) int {
	ppr := para.FindElement("pPr")
	if ppr == nil {
		return 0
	}
	styleElem := ppr.FindElement("pStyle")
	if styleElem == nil {
		return 0
	}
	val := styleElem.SelectAttrValue("val", "")
	if val == "" {
		for _, attr := range styleElem.Attr {
			if attr.Key == "val" {
				val = attr.Value
				break
			}
		}
	}
	level, ok := headingLevel(val)
	if !ok {
		return 0
	}
	return level
}

// isProtected checks whether a target anchor falls within any protected zone.
func isProtected(target StableAnchor, zones []ProtectedZone) bool {
	for _, z := range zones {
		if z.Start.Part != "" && z.Start.Part != target.Part {
			continue
		}
		// Match by ParaID if available
		if target.ParaID != "" {
			if target.ParaID == z.Start.ParaID || target.ParaID == z.End.ParaID {
				return true
			}
		}
		// Match by Bookmark
		if target.Bookmark != "" && target.Bookmark == z.Name {
			return true
		}
	}
	return false
}
