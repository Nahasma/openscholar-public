package template

import (
	"fmt"
	"strings"

	"github.com/beevik/etree"
	"github.com/Nahasma/openscholar-public/internal/docx"
)

// Analyze inspects a template document and returns its TemplateSpec,
// listing all fillable slots, protected zones, and section structure.
func Analyze(pkg *docx.Package) (*TemplateSpec, error) {
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("template: Analyze: %w", err)
	}

	spec := &TemplateSpec{
		Metadata: make(map[string]string),
	}

	// Track which slot names have already been seen (priority dedup).
	seen := make(map[string]bool)

	// ── Step 1: Content Control scan (highest priority) ─────────────────────
	if err := scanContentControls(doc, spec, seen); err != nil {
		return nil, fmt.Errorf("template: Analyze: content controls: %w", err)
	}

	// ── Step 2: Bookmark scan ────────────────────────────────────────────────
	if err := scanBookmarks(doc, spec, seen); err != nil {
		return nil, fmt.Errorf("template: Analyze: bookmarks: %w", err)
	}

	// ── Step 3: Placeholder scan (fallback) ─────────────────────────────────
	if err := scanPlaceholders(doc, spec, seen); err != nil {
		return nil, fmt.Errorf("template: Analyze: placeholders: %w", err)
	}

	// ── Step 4: Section structure ────────────────────────────────────────────
	spec.Sections = extractSections(doc)

	// ── Step 5: Metadata ─────────────────────────────────────────────────────
	spec.Metadata["slot_count"] = fmt.Sprintf("%d", len(spec.Slots))
	spec.Metadata["section_count"] = fmt.Sprintf("%d", countSections(spec.Sections))
	spec.Metadata["protected_zone_count"] = fmt.Sprintf("%d", len(spec.ProtectedZones))

	typeCounts := make(map[string]int)
	for _, s := range spec.Slots {
		typeCounts[slotDataTypeName(s.DataType)]++
	}
	for k, v := range typeCounts {
		spec.Metadata["slot_type_"+k] = fmt.Sprintf("%d", v)
	}

	return spec, nil
}

// scanContentControls finds all w:sdt elements and records their aliases as slots.
func scanContentControls(doc *etree.Document, spec *TemplateSpec, seen map[string]bool) error {
	root := doc.Root()
	if root == nil {
		return nil
	}

	var walk func(elem *etree.Element)
	walk = func(elem *etree.Element) {
		for _, child := range elem.ChildElements() {
			if child.Tag == "sdt" {
				// Extract alias from sdtPr/alias[@w:val]
				name := extractSDTAlias(child)
				if name != "" && !seen[name] {
					seen[name] = true
					anchor := docx.StableAnchor{
						Part: "word/document.xml",
						CCID: name,
					}
					spec.Slots = append(spec.Slots, SlotSpec{
						Name:       name,
						AnchorType: AnchorContentControl,
						DataType:   SlotText,
						Anchor:     anchor,
					})
				}
				// Recurse inside sdt content.
				walk(child)
			} else {
				walk(child)
			}
		}
	}
	walk(root)
	return nil
}

// extractSDTAlias pulls the alias name out of an sdt element.
func extractSDTAlias(sdt *etree.Element) string {
	sdtPr := sdt.FindElement("sdtPr")
	if sdtPr == nil {
		return ""
	}
	alias := sdtPr.FindElement("alias")
	if alias == nil {
		return ""
	}
	// Try val attribute — etree strips namespace prefix from key in SelectAttrValue.
	val := alias.SelectAttrValue("val", "")
	if val == "" {
		for _, attr := range alias.Attr {
			if attr.Key == "val" {
				val = attr.Value
				break
			}
		}
	}
	return strings.TrimSpace(val)
}

// scanBookmarks finds all bookmarkStart elements and records them as slots.
func scanBookmarks(doc *etree.Document, spec *TemplateSpec, seen map[string]bool) error {
	root := doc.Root()
	if root == nil {
		return nil
	}

	var walk func(elem *etree.Element)
	walk = func(elem *etree.Element) {
		for _, child := range elem.ChildElements() {
			if child.Tag == "bookmarkStart" {
				name := child.SelectAttrValue("name", "")
				if name == "" {
					for _, attr := range child.Attr {
						if attr.Key == "name" {
							name = attr.Value
							break
						}
					}
				}
				// Skip built-in bookmarks (start with "_") and already-seen names.
				if name != "" && !strings.HasPrefix(name, "_") && !seen[name] {
					seen[name] = true
					anchor := docx.StableAnchor{
						Part:     "word/document.xml",
						Bookmark: name,
					}
					spec.Slots = append(spec.Slots, SlotSpec{
						Name:       name,
						AnchorType: AnchorBookmark,
						DataType:   SlotText,
						Anchor:     anchor,
					})
				}
			}
			walk(child)
		}
	}
	walk(root)
	return nil
}

// scanPlaceholders walks all paragraphs and finds {{...}} style placeholders.
func scanPlaceholders(doc *etree.Document, spec *TemplateSpec, seen map[string]bool) error {
	syntax := docx.DefaultSyntax()

	// We need to track loop/cond start markers so we can find matching ends.
	// For loop: {{#key}} ... {{/key}} → SlotLoop
	// For cond: {{?key}} ... {{/key}} → SlotConditional
	// For protected: {{!key}} → ProtectedZone (no Slot)

	err := docx.Traverse(doc, func(elem *etree.Element, _ int) error {
		if elem.Tag == "p" {
			processParagraphPlaceholders(elem, syntax, spec, seen)
		} else {
			// Recurse into tables and other containers.
			traverseParagraphsIn(elem, func(para *etree.Element) {
				processParagraphPlaceholders(para, syntax, spec, seen)
			})
		}
		return nil
	})
	return err
}

// processParagraphPlaceholders scans one paragraph for placeholders and
// updates spec/seen accordingly.
func processParagraphPlaceholders(para *etree.Element, syntax docx.PlaceholderSyntax, spec *TemplateSpec, seen map[string]bool) {
	matches := docx.FindPlaceholders(para, syntax)
	for _, m := range matches {
		switch m.Type {
		case docx.PTProtected:
			// Protected zone — not a slot, record separately.
			// We record each {{!key}} as both start and end of a zone
			// (single-paragraph zones for now).
			anchor := docx.StableAnchor{Part: "word/document.xml"}
			spec.ProtectedZones = append(spec.ProtectedZones, ProtectedZone{
				Name:  m.Key,
				Start: anchor,
				End:   anchor,
			})

		case docx.PTLoopStart:
			if !seen[m.Key] {
				seen[m.Key] = true
				spec.Slots = append(spec.Slots, SlotSpec{
					Name:       m.Key,
					AnchorType: AnchorPlaceholder,
					DataType:   SlotLoop,
					Anchor:     docx.StableAnchor{Part: "word/document.xml"},
				})
			}

		case docx.PTCondStart:
			if !seen[m.Key] {
				seen[m.Key] = true
				spec.Slots = append(spec.Slots, SlotSpec{
					Name:       m.Key,
					AnchorType: AnchorPlaceholder,
					DataType:   SlotConditional,
					Anchor:     docx.StableAnchor{Part: "word/document.xml"},
				})
			}

		case docx.PTImage:
			if !seen[m.Key] {
				seen[m.Key] = true
				spec.Slots = append(spec.Slots, SlotSpec{
					Name:       m.Key,
					AnchorType: AnchorPlaceholder,
					DataType:   SlotImage,
					Anchor:     docx.StableAnchor{Part: "word/document.xml"},
				})
			}

		case docx.PTVariable, docx.PTExpression:
			if !seen[m.Key] {
				seen[m.Key] = true
				spec.Slots = append(spec.Slots, SlotSpec{
					Name:       m.Key,
					AnchorType: AnchorPlaceholder,
					DataType:   SlotText,
					Anchor:     docx.StableAnchor{Part: "word/document.xml"},
				})
			}

		case docx.PTLoopEnd, docx.PTCondEnd:
			// End markers are not slots themselves — they were already
			// recorded when the start marker was seen.
		}
	}
}

// traverseParagraphsIn recursively finds w:p elements inside elem and calls fn.
func traverseParagraphsIn(elem *etree.Element, fn func(*etree.Element)) {
	for _, child := range elem.ChildElements() {
		if child.Tag == "p" {
			fn(child)
		} else {
			traverseParagraphsIn(child, fn)
		}
	}
}

// extractSections walks the document body and builds a SectionSpec tree
// from heading paragraphs.
func extractSections(doc *etree.Document) []SectionSpec {
	root := doc.Root()
	if root == nil {
		return nil
	}

	// Find body.
	var body *etree.Element
	for _, child := range root.ChildElements() {
		if child.Tag == "body" {
			body = child
			break
		}
	}
	if body == nil {
		return nil
	}

	// Flat list of headings with levels.
	var flat []SectionSpec

	for _, child := range body.ChildElements() {
		if child.Tag != "p" {
			continue
		}
		level, title := extractHeading(child)
		if level == 0 {
			continue
		}
		paraID := ""
		for _, attr := range child.Attr {
			if attr.Key == "paraId" {
				paraID = attr.Value
				break
			}
		}
		flat = append(flat, SectionSpec{
			Title: title,
			Level: level,
			Anchor: docx.StableAnchor{
				Part:   "word/document.xml",
				ParaID: paraID,
			},
		})
	}

	// Build tree from flat list.
	return buildSectionTree(flat)
}

func buildSectionTree(flat []SectionSpec) []SectionSpec {
	if len(flat) == 0 {
		return nil
	}

	var result []SectionSpec
	// Stack of (section pointer, level) for building the tree.
	type stackEntry struct {
		section *SectionSpec
		level   int
	}
	var stack []stackEntry

	for i := range flat {
		sec := flat[i] // copy
		sec.Children = nil

		for len(stack) > 0 && stack[len(stack)-1].level >= sec.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			result = append(result, sec)
			// Push a pointer to the last element of result.
			stack = append(stack, stackEntry{section: &result[len(result)-1], level: sec.Level})
		} else {
			parent := stack[len(stack)-1].section
			parent.Children = append(parent.Children, sec)
			// Push a pointer to the last child.
			stack = append(stack, stackEntry{
				section: &parent.Children[len(parent.Children)-1],
				level:   sec.Level,
			})
		}
	}

	return result
}

// extractHeading returns the heading level (1-9) and text for a w:p element.
// Returns 0 if the paragraph is not a heading.
func extractHeading(para *etree.Element) (level int, title string) {
	ppr := para.FindElement("pPr")
	if ppr == nil {
		return 0, ""
	}
	styleElem := ppr.FindElement("pStyle")
	if styleElem == nil {
		return 0, ""
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

	level = parseHeadingLevel(val)
	if level == 0 {
		return 0, ""
	}

	// Extract text content from runs.
	var sb strings.Builder
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			for _, rc := range child.ChildElements() {
				if rc.Tag == "t" {
					sb.WriteString(rc.Text())
				}
			}
		}
	}
	return level, sb.String()
}

// parseHeadingLevel returns 1-9 for heading styles, or 0 if not a heading.
func parseHeadingLevel(styleVal string) int {
	lower := strings.ToLower(strings.TrimSpace(styleVal))
	if strings.HasPrefix(lower, "heading") {
		suffix := strings.TrimLeft(lower[len("heading"):], " -_")
		n := 0
		if _, err := fmt.Sscanf(suffix, "%d", &n); err == nil && n >= 1 && n <= 9 {
			return n
		}
		if suffix == "" {
			return 1
		}
	}
	return 0
}

// countSections recursively counts all SectionSpec nodes.
func countSections(sections []SectionSpec) int {
	total := len(sections)
	for _, s := range sections {
		total += countSections(s.Children)
	}
	return total
}

// slotDataTypeName returns a short name for a SlotDataType (used in metadata keys).
func slotDataTypeName(dt SlotDataType) string {
	switch dt {
	case SlotText:
		return "text"
	case SlotRichText:
		return "richtext"
	case SlotImage:
		return "image"
	case SlotTable:
		return "table"
	case SlotConditional:
		return "conditional"
	case SlotLoop:
		return "loop"
	default:
		return "unknown"
	}
}
