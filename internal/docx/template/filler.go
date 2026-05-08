package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/beevik/etree"
	"github.com/Nahasma/openscholar-public/internal/docx"
)

// FillResult describes the outcome of a template fill operation.
type FillResult struct {
	Replaced []docx.ReplacementResult
	Warnings []string // Unmatched slots, type mismatches, etc.
	Errors   []string // Missing required slots
}

// Fill populates a template with the given data according to the spec.
func Fill(pkg *docx.Package, spec *TemplateSpec, data map[string]any) (*FillResult, error) {
	result := &FillResult{}

	// ── Check required slots ─────────────────────────────────────────────────
	for _, slot := range spec.Slots {
		if slot.Required {
			if _, ok := data[slot.Name]; !ok {
				result.Errors = append(result.Errors,
					fmt.Sprintf("required slot %q is missing from data", slot.Name))
			}
		}
	}

	// ── Warn about data keys that have no matching slot ──────────────────────
	slotNames := make(map[string]bool, len(spec.Slots))
	for _, s := range spec.Slots {
		slotNames[s.Name] = true
	}
	for k := range data {
		if !slotNames[k] {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("data key %q has no matching slot in template", k))
		}
	}

	// ── Process slots by type ────────────────────────────────────────────────
	for _, slot := range spec.Slots {
		val, hasVal := data[slot.Name]
		if !hasVal {
			if slot.Default != "" {
				val = slot.Default
				hasVal = true
			}
		}

		switch slot.DataType {
		case SlotText, SlotRichText:
			// Handled in bulk via ReplaceAll below.

		case SlotLoop:
			if !hasVal {
				continue
			}
			rows, ok := toRowSlice(val)
			if !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slot %q expects []map[string]any for loop, got %T", slot.Name, val))
				continue
			}
			if err := expandLoop(pkg, slot.Name, rows); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slot %q loop expansion failed: %v", slot.Name, err))
			}

		case SlotConditional:
			if !hasVal {
				// Default: hide conditional block when no data.
				if err := applyConditional(pkg, slot.Name, false); err != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("slot %q conditional failed: %v", slot.Name, err))
				}
				continue
			}
			show := toBool(val)
			if err := applyConditional(pkg, slot.Name, show); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slot %q conditional failed: %v", slot.Name, err))
			}

		case SlotImage:
			if !hasVal {
				continue
			}
			imgData, ok := toImageData(val)
			if !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slot %q expects ImageData or string path for image, got %T", slot.Name, val))
				continue
			}
			if err := insertImage(pkg, slot.Name, imgData, result); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slot %q image insertion failed: %v", slot.Name, err))
			}
		}
	}

	// ── Variable (text) replacement via ReplaceAll ───────────────────────────
	textData := make(map[string]any)
	for _, slot := range spec.Slots {
		if slot.DataType != SlotText && slot.DataType != SlotRichText {
			continue
		}
		val, hasVal := data[slot.Name]
		if !hasVal {
			if slot.Default != "" {
				textData[slot.Name] = slot.Default
			}
			continue
		}
		textData[slot.Name] = fmt.Sprint(val)
	}

	if len(textData) > 0 {
		replaced, err := docx.ReplaceAll(pkg, textData)
		if err != nil {
			return result, fmt.Errorf("template: Fill: ReplaceAll: %w", err)
		}
		result.Replaced = append(result.Replaced, replaced...)
	}

	return result, nil
}

// ── Loop expansion ───────────────────────────────────────────────────────────

// expandLoop finds the {{#key}} ... {{/key}} block and expands it.
func expandLoop(pkg *docx.Package, key string, rows []map[string]any) error {
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return err
	}

	body := findBody(doc)
	if body == nil {
		return fmt.Errorf("no w:body found")
	}

	children := body.ChildElements()
	startIdx, endIdx := -1, -1

	syntax := docx.DefaultSyntax()
	startTag := syntax.Open + "#" + key + syntax.Close
	endTag := syntax.Open + "/" + key + syntax.Close

	for i, child := range children {
		if child.Tag != "p" {
			continue
		}
		text := paragraphFullText(child)
		if startIdx == -1 && strings.Contains(text, startTag) {
			startIdx = i
		} else if startIdx != -1 && endIdx == -1 && strings.Contains(text, endTag) {
			endIdx = i
			break
		}
	}

	if startIdx == -1 || endIdx == -1 {
		// Placeholder not found — not an error (template may not have the loop).
		return nil
	}

	// The template paragraphs between start and end (exclusive).
	templateParas := children[startIdx+1 : endIdx]

	// For each row, clone the template paragraphs and replace variables.
	var insertedElems []*etree.Element
	for _, row := range rows {
		for _, tmpl := range templateParas {
			clone := tmpl.Copy()
			// Replace placeholders inside this clone.
			replaceInElement(clone, syntax, row)
			insertedElems = append(insertedElems, clone)
		}
	}

	// Remove start, template, and end paragraphs from body, insert expanded ones.
	// Build a new child token list.
	var newTokens []etree.Token
	insertDone := false
	for _, tok := range body.Child {
		elem, isElem := tok.(*etree.Element)
		if !isElem {
			newTokens = append(newTokens, tok)
			continue
		}

		// Check if this element is the start marker.
		idx := elemIndex(children, elem)
		if idx == startIdx {
			// Insert expanded elements here (before removing start).
			if !insertDone {
				for _, e := range insertedElems {
					newTokens = append(newTokens, e)
				}
				insertDone = true
			}
			// Skip start marker.
			continue
		}
		if idx >= startIdx+1 && idx <= endIdx {
			// Skip template paragraphs and end marker.
			continue
		}
		newTokens = append(newTokens, tok)
	}
	body.Child = newTokens

	return pkg.SetXML("word/document.xml", doc)
}

// replaceInElement replaces all placeholders in elem recursively using data.
func replaceInElement(elem *etree.Element, syntax docx.PlaceholderSyntax, data map[string]any) {
	if elem.Tag == "p" {
		matches := docx.FindPlaceholders(elem, syntax)
		// Apply back to front.
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			if m.Type == docx.PTVariable || m.Type == docx.PTExpression {
				if val, ok := data[m.Key]; ok {
					_ = docx.Replace(elem, m, fmt.Sprint(val))
				}
			}
		}
		return
	}
	for _, child := range elem.ChildElements() {
		replaceInElement(child, syntax, data)
	}
}

// elemIndex finds the index of elem in a slice of elements.
func elemIndex(elems []*etree.Element, elem *etree.Element) int {
	for i, e := range elems {
		if e == elem {
			return i
		}
	}
	return -1
}

// paragraphFullText returns the concatenated text of all w:t elements in a paragraph.
func paragraphFullText(para *etree.Element) string {
	var sb strings.Builder
	var walk func(e *etree.Element)
	walk = func(e *etree.Element) {
		if e.Tag == "t" {
			sb.WriteString(e.Text())
		}
		for _, child := range e.ChildElements() {
			walk(child)
		}
	}
	walk(para)
	return sb.String()
}

// findBody finds the w:body element inside the document root.
func findBody(doc *etree.Document) *etree.Element {
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

// ── Conditional ──────────────────────────────────────────────────────────────

// applyConditional shows or hides the block between {{?key}} and {{/key}}.
func applyConditional(pkg *docx.Package, key string, show bool) error {
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return err
	}

	body := findBody(doc)
	if body == nil {
		return fmt.Errorf("no w:body found")
	}

	children := body.ChildElements()
	syntax := docx.DefaultSyntax()
	startTag := syntax.Open + "?" + key + syntax.Close
	endTag := syntax.Open + "/" + key + syntax.Close

	startIdx, endIdx := -1, -1
	for i, child := range children {
		if child.Tag != "p" {
			continue
		}
		text := paragraphFullText(child)
		if startIdx == -1 && strings.Contains(text, startTag) {
			startIdx = i
		} else if startIdx != -1 && endIdx == -1 && strings.Contains(text, endTag) {
			endIdx = i
			break
		}
	}

	if startIdx == -1 || endIdx == -1 {
		return nil
	}

	if show {
		// Remove only the control markers (start and end paragraphs), keep content.
		var newTokens []etree.Token
		for _, tok := range body.Child {
			elem, isElem := tok.(*etree.Element)
			if !isElem {
				newTokens = append(newTokens, tok)
				continue
			}
			idx := elemIndex(children, elem)
			if idx == startIdx || idx == endIdx {
				continue // Remove marker paragraphs.
			}
			newTokens = append(newTokens, tok)
		}
		body.Child = newTokens
	} else {
		// Remove everything from start to end (inclusive).
		var newTokens []etree.Token
		for _, tok := range body.Child {
			elem, isElem := tok.(*etree.Element)
			if !isElem {
				newTokens = append(newTokens, tok)
				continue
			}
			idx := elemIndex(children, elem)
			if idx >= startIdx && idx <= endIdx {
				continue
			}
			newTokens = append(newTokens, tok)
		}
		body.Child = newTokens
	}

	return pkg.SetXML("word/document.xml", doc)
}

// ── Image insertion ───────────────────────────────────────────────────────────

// insertImage replaces {{img:key}} with a w:drawing element.
func insertImage(pkg *docx.Package, key string, img ImageData, result *FillResult) error {
	// Read image bytes.
	imgBytes, err := os.ReadFile(img.Path)
	if err != nil {
		return fmt.Errorf("read image %q: %w", img.Path, err)
	}

	// Determine extension and content type.
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(img.Path)), ".")
	if ext == "" {
		ext = "png"
	}
	ct := extensionToContentType(ext)

	// Add the image as a new Part.
	imgURI := "word/media/" + key + "." + ext
	pkg.SetPart(imgURI, imgBytes)

	// Ensure content type is registered.
	if err := docx.EnsureContentType(pkg, ext, ct); err != nil {
		return fmt.Errorf("EnsureContentType: %w", err)
	}

	// Load and update relations for word/document.xml.
	rels, err := docx.LoadRelations(pkg, "word/document.xml")
	if err != nil {
		return fmt.Errorf("LoadRelations: %w", err)
	}
	// The image target path is relative to word/.
	rID := rels.AddImage("media/" + key + "." + ext)
	if err := rels.Save(pkg); err != nil {
		return fmt.Errorf("rels.Save: %w", err)
	}

	// Determine dimensions (default to 1 inch × 1 inch if not specified).
	width := img.Width
	height := img.Height
	if width == 0 {
		width = 914400 // 1 inch in EMU
	}
	if height == 0 {
		height = 914400
	}

	// Build the w:drawing XML.
	drawingXML := buildDrawingXML(rID, width, height)

	// Replace the {{img:key}} placeholder in the document.
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return err
	}

	syntax := docx.DefaultSyntax()
	placeholder := syntax.Open + "img:" + key + syntax.Close

	modified := false
	err = docx.Traverse(doc, func(elem *etree.Element, _ int) error {
		if elem.Tag == "p" {
			if replaceImgPlaceholder(elem, placeholder, drawingXML) {
				modified = true
			}
		} else {
			traverseParagraphsIn(elem, func(para *etree.Element) {
				if replaceImgPlaceholder(para, placeholder, drawingXML) {
					modified = true
				}
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	if modified {
		if err := pkg.SetXML("word/document.xml", doc); err != nil {
			return err
		}
		result.Replaced = append(result.Replaced, docx.ReplacementResult{
			Placeholder: placeholder,
			NewValue:    "[image:" + rID + "]",
			Success:     true,
		})
	}

	return nil
}

// replaceImgPlaceholder replaces an image placeholder in a paragraph by
// clearing the paragraph's run content and inserting a drawing element.
func replaceImgPlaceholder(para *etree.Element, placeholder string, drawingXML string) bool {
	text := paragraphFullText(para)
	if !strings.Contains(text, placeholder) {
		return false
	}

	// Parse the drawing XML fragment.
	drawDoc := etree.NewDocument()
	// Wrap in a temporary root to parse.
	wrapped := `<w:r xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
		xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
		xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
		xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"
		xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		drawingXML + `</w:r>`
	if err := drawDoc.ReadFromString(wrapped); err != nil {
		return false
	}
	drawRoot := drawDoc.Root()
	if drawRoot == nil {
		return false
	}

	// Remove all existing runs from the paragraph.
	var newChild []etree.Token
	for _, tok := range para.Child {
		if elem, ok := tok.(*etree.Element); ok && elem.Tag == "r" {
			continue
		}
		newChild = append(newChild, tok)
	}

	// Add a run containing the drawing.
	newRun := etree.NewElement("r")
	newRun.Space = "w"
	for _, child := range drawRoot.ChildElements() {
		newRun.AddChild(child.Copy())
	}
	newChild = append(newChild, newRun)
	para.Child = newChild

	return true
}

// buildDrawingXML constructs a minimal w:drawing XML string for an inline image.
func buildDrawingXML(rID string, width, height int) string {
	return fmt.Sprintf(`<w:drawing xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <wp:inline distT="0" distB="0" distL="0" distR="0"
    xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">
    <wp:extent cx="%d" cy="%d"/>
    <wp:docPr id="1" name="Picture"/>
    <a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
      <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">
        <pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
          <pic:blipFill>
            <a:blip r:embed="%s"
              xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/>
          </pic:blipFill>
          <pic:spPr>
            <a:xfrm>
              <a:ext cx="%d" cy="%d"/>
            </a:xfrm>
          </pic:spPr>
        </pic:pic>
      </a:graphicData>
    </a:graphic>
  </wp:inline>
</w:drawing>`, width, height, rID, width, height)
}

// extensionToContentType maps common image extensions to MIME types.
func extensionToContentType(ext string) string {
	switch ext {
	case "png":
		return docx.ContentTypePNG
	case "jpg", "jpeg":
		return docx.ContentTypeJPEG
	case "gif":
		return docx.ContentTypeGIF
	default:
		return "image/" + ext
	}
}

// ── Type helpers ──────────────────────────────────────────────────────────────

// toRowSlice converts a value to []map[string]any if possible.
func toRowSlice(v any) ([]map[string]any, bool) {
	switch t := v.(type) {
	case []map[string]any:
		return t, true
	case []any:
		rows := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				rows = append(rows, m)
			} else {
				return nil, false
			}
		}
		return rows, true
	}
	return nil, false
}

// toBool converts a value to bool following common conventions.
func toBool(v any) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s != "" && s != "false" && s != "0" && s != "no"
	}
	return false
}

// toImageData converts a value to ImageData.
func toImageData(v any) (ImageData, bool) {
	switch t := v.(type) {
	case ImageData:
		return t, true
	case string:
		return ImageData{Path: t}, true
	}
	return ImageData{}, false
}
