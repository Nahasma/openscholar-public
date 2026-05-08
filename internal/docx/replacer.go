package docx

import (
	"fmt"
	"strings"

	"github.com/beevik/etree"
)

// ReplacementResult records one replacement's outcome.
type ReplacementResult struct {
	Placeholder string
	Anchor      StableAnchor
	OldValue    string
	NewValue    string
	Success     bool
	Error       string
}

// Replace replaces a single placeholder match in its paragraph
// with newContent, preserving the original Run's formatting (w:rPr).
//
// Algorithm:
//  1. Extract the rPr (run properties) from the first matched Run to use as style template.
//  2. Trim the start of StartRun's text to the portion before StartOffset.
//  3. Create a new Run with the copied rPr and newContent as w:t text.
//  4. Trim the end of EndRun's text to the portion after EndOffset.
//  5. Remove all intermediate Runs (StartRun+1 … EndRun-1).
//  6. Insert the new Run in the correct position.
func Replace(para *etree.Element, match PlaceholderMatch, newContent string) error {
	if len(match.Runs) == 0 {
		return fmt.Errorf("docx: Replace: match has no runs")
	}

	startRunElem := match.Runs[0]
	endRunElem := match.Runs[len(match.Runs)-1]

	// ── 1. Clone rPr from the first run ────────────────────────────────────────
	var rPrClone *etree.Element
	if rpr := startRunElem.FindElement("rPr"); rpr != nil {
		rPrClone = rpr.Copy()
	}

	// ── 2. Trim start run text ─────────────────────────────────────────────────
	startText := runText(startRunElem)
	startRunes := []rune(startText)
	prefixText := string(startRunes[:match.StartOffset])

	// ── 3. Trim end run text ───────────────────────────────────────────────────
	endText := runText(endRunElem)
	endRunes := []rune(endText)
	// EndOffset is exclusive rune count from start of that run.
	var suffixText string
	if match.EndOffset <= len(endRunes) {
		suffixText = string(endRunes[match.EndOffset:])
	}

	// ── 4. Build the replacement run ───────────────────────────────────────────
	newRun := etree.NewElement("r")
	newRun.Space = "w"
	if rPrClone != nil {
		newRun.AddChild(rPrClone)
	}
	newT := newRun.CreateElement("t")
	newT.Space = "w"
	newT.SetText(newContent)
	// Preserve leading/trailing whitespace with xml:space="preserve".
	if len(newContent) > 0 && (newContent[0] == ' ' || newContent[len(newContent)-1] == ' ') {
		newT.CreateAttr("xml:space", "preserve")
	}

	// ── 5. Locate the parent index of startRunElem ─────────────────────────────
	// We need to know where to insert and what to remove.
	children := para.Child
	startIdx := -1
	for i, tok := range children {
		if elem, ok := tok.(*etree.Element); ok && elem == startRunElem {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return fmt.Errorf("docx: Replace: start run not found in paragraph")
	}

	// ── 6. Update start run's text ─────────────────────────────────────────────
	setRunText(startRunElem, prefixText)

	// ── 7. Handle single-run vs multi-run case ─────────────────────────────────
	if match.StartRun == match.EndRun {
		// Single run: split into [prefix run] [new run] [suffix run].
		// Update the original run with prefix text; insert new run and suffix run after it.
		insertAfterToken(para, startRunElem, buildSuffixRun(rPrClone, suffixText))
		insertAfterToken(para, startRunElem, newRun)
	} else {
		// Multi-run: remove all runs between start and end (exclusive), then handle end run.
		// First update end run text to suffix.
		setRunText(endRunElem, suffixText)

		// Remove intermediate runs (those between StartRun and EndRun in match.Runs).
		for i := 1; i < len(match.Runs)-1; i++ {
			para.RemoveChild(match.Runs[i])
		}

		// Insert the new run just after the (already updated) start run.
		insertAfterToken(para, startRunElem, newRun)
	}

	return nil
}

// setRunText updates the w:t text inside a w:r element.
// Preserves xml:space="preserve" for whitespace-bearing text.
func setRunText(run *etree.Element, text string) {
	// Remove existing w:t elements.
	for _, child := range run.ChildElements() {
		if child.Tag == "t" {
			run.RemoveChild(child)
		}
	}
	if text == "" {
		// Leave an empty w:t so the run remains structurally valid.
		t := run.CreateElement("t")
		t.Space = "w"
		return
	}
	t := run.CreateElement("t")
	t.Space = "w"
	t.SetText(text)
	if text[0] == ' ' || text[len(text)-1] == ' ' {
		t.CreateAttr("xml:space", "preserve")
	}
}

// buildSuffixRun creates a new w:r element carrying the given text and
// an optional cloned rPr. Returns nil when text is empty (caller may skip).
func buildSuffixRun(rPrClone *etree.Element, text string) *etree.Element {
	if text == "" {
		return nil
	}
	r := etree.NewElement("r")
	r.Space = "w"
	if rPrClone != nil {
		r.AddChild(rPrClone.Copy())
	}
	t := r.CreateElement("t")
	t.Space = "w"
	t.SetText(text)
	if text[0] == ' ' || text[len(text)-1] == ' ' {
		t.CreateAttr("xml:space", "preserve")
	}
	return r
}

// insertAfterToken inserts newElem immediately after ref in parent's child list.
// If newElem is nil, it does nothing.
func insertAfterToken(parent *etree.Element, ref *etree.Element, newElem *etree.Element) {
	if newElem == nil {
		return
	}
	// Find ref's index among all child tokens.
	idx := -1
	for i, tok := range parent.Child {
		if elem, ok := tok.(*etree.Element); ok && elem == ref {
			idx = i
			break
		}
	}
	if idx < 0 {
		parent.AddChild(newElem)
		return
	}
	// Build new child slice with newElem inserted at idx+1.
	newChildren := make([]etree.Token, 0, len(parent.Child)+1)
	newChildren = append(newChildren, parent.Child[:idx+1]...)
	newChildren = append(newChildren, newElem)
	newChildren = append(newChildren, parent.Child[idx+1:]...)
	parent.Child = newChildren
}

// ReplaceAll finds and replaces all placeholders in the document package.
// It traverses word/document.xml, finds every paragraph, locates all
// PlaceholderMatch instances using DefaultSyntax, and replaces them
// from the data map.
//
// Keys in data are matched against PlaceholderMatch.Key.
// Values are converted to strings via fmt.Sprint.
// Missing keys are recorded as errors in the result but do not abort processing.
// Replacements within a paragraph are applied back-to-front to preserve offsets.
func ReplaceAll(pkg *Package, data map[string]any) ([]ReplacementResult, error) {
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("docx: ReplaceAll: %w", err)
	}

	syntax := DefaultSyntax()
	var results []ReplacementResult

	err = Traverse(doc, func(elem *etree.Element, _ int) error {
		if elem.Tag != "p" {
			// Also search inside table cells.
			traverseParagraphsInElem(elem, func(para *etree.Element) {
				r := replaceParagraph(para, syntax, data)
				results = append(results, r...)
			})
			return nil
		}
		r := replaceParagraph(elem, syntax, data)
		results = append(results, r...)
		return nil
	})
	if err != nil {
		return results, fmt.Errorf("docx: ReplaceAll traverse: %w", err)
	}

	// Persist the mutated DOM back to the part.
	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		return results, fmt.Errorf("docx: ReplaceAll: SetXML: %w", err)
	}

	return results, nil
}

// replaceParagraph performs placeholder replacement for a single w:p element
// and returns the results. Replacements are applied right-to-left within each
// paragraph to avoid index shift issues.
func replaceParagraph(para *etree.Element, syntax PlaceholderSyntax, data map[string]any) []ReplacementResult {
	matches := FindPlaceholders(para, syntax)
	if len(matches) == 0 {
		return nil
	}

	var results []ReplacementResult

	// Apply in reverse order to avoid positional drift.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		val, ok := data[m.Key]

		res := ReplacementResult{
			Placeholder: m.FullText,
			OldValue:    m.FullText,
		}

		if !ok {
			res.Success = false
			res.Error = fmt.Sprintf("key %q not found in data", m.Key)
			results = append(results, res)
			continue
		}

		newContent := fmt.Sprint(val)
		res.NewValue = newContent

		if err := Replace(para, m, newContent); err != nil {
			res.Success = false
			res.Error = err.Error()
		} else {
			res.Success = true
		}
		results = append(results, res)
	}

	return results
}

// traverseParagraphsInElem recursively finds w:p elements inside elem
// (used for table cells and other containers) and calls fn for each.
func traverseParagraphsInElem(elem *etree.Element, fn func(*etree.Element)) {
	for _, child := range elem.ChildElements() {
		if child.Tag == "p" {
			fn(child)
		} else {
			traverseParagraphsInElem(child, fn)
		}
	}
}

// paragraphText is a helper used in tests to read the full text of a paragraph.
func paragraphText(para *etree.Element) string {
	var sb strings.Builder
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			sb.WriteString(runText(child))
		}
	}
	return sb.String()
}
