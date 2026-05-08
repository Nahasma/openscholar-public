package docx

import (
	"fmt"
)

// Common content types
const (
	ContentTypeXML      = "application/xml"
	ContentTypePNG      = "image/png"
	ContentTypeJPEG     = "image/jpeg"
	ContentTypeGIF      = "image/gif"
	ContentTypeRels     = "application/vnd.openxmlformats-package.relationships+xml"
	ContentTypeDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
)

// extensionContentTypes maps file extensions to their default content types.
var extensionContentTypes = map[string]string{
	"png":  ContentTypePNG,
	"jpg":  ContentTypeJPEG,
	"jpeg": ContentTypeJPEG,
	"gif":  ContentTypeGIF,
	"xml":  ContentTypeXML,
	"rels": ContentTypeRels,
}

// EnsureContentType checks that [Content_Types].xml contains a Default
// entry for the given file extension. If missing, it adds one.
func EnsureContentType(pkg *Package, ext string, contentType string) error {
	const ctURI = "[Content_Types].xml"

	doc, err := pkg.XML(ctURI)
	if err != nil {
		return fmt.Errorf("docx: EnsureContentType: %w", err)
	}

	root := doc.Root()
	if root == nil {
		return fmt.Errorf("docx: EnsureContentType: %s has no root element", ctURI)
	}

	// Check for an existing Default entry with the same extension.
	for _, elem := range root.ChildElements() {
		if elem.Tag == "Default" && elem.SelectAttrValue("Extension", "") == ext {
			return nil
		}
	}

	// Add a new Default entry.
	elem := root.CreateElement("Default")
	elem.CreateAttr("Extension", ext)
	elem.CreateAttr("ContentType", contentType)

	return pkg.SetXML(ctURI, doc)
}

// EnsureOverride checks that [Content_Types].xml contains an Override
// entry for the given part name. If missing, it adds one.
func EnsureOverride(pkg *Package, partName string, contentType string) error {
	const ctURI = "[Content_Types].xml"

	doc, err := pkg.XML(ctURI)
	if err != nil {
		return fmt.Errorf("docx: EnsureOverride: %w", err)
	}

	root := doc.Root()
	if root == nil {
		return fmt.Errorf("docx: EnsureOverride: %s has no root element", ctURI)
	}

	// Check for an existing Override entry with the same PartName.
	for _, elem := range root.ChildElements() {
		if elem.Tag == "Override" && elem.SelectAttrValue("PartName", "") == partName {
			return nil
		}
	}

	// Add a new Override entry.
	elem := root.CreateElement("Override")
	elem.CreateAttr("PartName", partName)
	elem.CreateAttr("ContentType", contentType)

	return pkg.SetXML(ctURI, doc)
}

// lookupExtensionContentType returns the default content type for a file
// extension, or empty string if unknown.
func lookupExtensionContentType(ext string) string {
	return extensionContentTypes[ext]
}

// ensureDefaultContentTypes registers the standard Default entries
// (rels, xml) that every OOXML package needs.
func ensureDefaultContentTypes(pkg *Package) error {
	defaults := []struct{ ext, ct string }{
		{"rels", ContentTypeRels},
		{"xml", ContentTypeXML},
	}
	for _, d := range defaults {
		if err := EnsureContentType(pkg, d.ext, d.ct); err != nil {
			return err
		}
	}
	return nil
}

