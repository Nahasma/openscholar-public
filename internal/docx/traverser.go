package docx

import "github.com/beevik/etree"

// Traverse walks the direct children of w:body in document.xml,
// calling fn for each top-level element with its index.
func Traverse(doc *etree.Document, fn func(elem *etree.Element, index int) error) error {
	root := doc.Root()
	if root == nil {
		return nil
	}

	// Find w:body — etree stores the local name in Tag (no prefix).
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

	for i, child := range body.ChildElements() {
		if err := fn(child, i); err != nil {
			return err
		}
	}
	return nil
}

// TraversePart opens the named XML part from pkg and traverses its
// root element's children (for headers, footers, etc.).
func TraversePart(pkg *Package, partURI string, fn func(elem *etree.Element, index int) error) error {
	doc, err := pkg.XML(partURI)
	if err != nil {
		return err
	}
	root := doc.Root()
	if root == nil {
		return nil
	}
	for i, child := range root.ChildElements() {
		if err := fn(child, i); err != nil {
			return err
		}
	}
	return nil
}
