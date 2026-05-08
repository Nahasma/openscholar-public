package docx

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// Relationship describes a single entry in a .rels file.
type Relationship struct {
	ID         string // e.g. "rId1"
	Type       string // relationship type URI
	Target     string // target path
	TargetMode string // "Internal" (default) or "External"
}

// Common relationship type URIs
const (
	RelTypeImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	RelTypeHyperlink = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	RelTypeHeader    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	RelTypeFooter    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	RelTypeStyles    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	RelTypeDocument  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
)

// Relations manages the .rels file for a specific Part.
type Relations struct {
	partURI string                  // e.g. "word/document.xml"
	rels    map[string]Relationship // ID → Relationship
	nextID  int                     // next available rId number
}

// relsURI returns the .rels file path for a given part URI.
// e.g. "word/document.xml" → "word/_rels/document.xml.rels"
//
//	"[Content_Types].xml" → "_rels/.rels" (root rels)
func relsURI(partURI string) string {
	dir := path.Dir(partURI)
	base := path.Base(partURI)
	if dir == "." {
		// Root-level part: no directory component.
		return "_rels/" + base + ".rels"
	}
	return dir + "/_rels/" + base + ".rels"
}

// LoadRelations reads the .rels file for the given part URI from the package.
// If no .rels file exists, returns an empty Relations.
func LoadRelations(pkg *Package, partURI string) (*Relations, error) {
	r := &Relations{
		partURI: partURI,
		rels:    make(map[string]Relationship),
		nextID:  1,
	}

	uri := relsURI(partURI)
	if !pkg.HasPart(uri) {
		return r, nil
	}

	doc, err := pkg.XML(uri)
	if err != nil {
		return nil, fmt.Errorf("docx: load relations for %q: %w", partURI, err)
	}

	root := doc.Root()
	if root == nil {
		return r, nil
	}

	maxID := 0
	for _, elem := range root.ChildElements() {
		if elem.Tag != "Relationship" {
			continue
		}
		rel := Relationship{
			ID:         elem.SelectAttrValue("Id", ""),
			Type:       elem.SelectAttrValue("Type", ""),
			Target:     elem.SelectAttrValue("Target", ""),
			TargetMode: elem.SelectAttrValue("TargetMode", ""),
		}
		r.rels[rel.ID] = rel

		// Parse the numeric suffix of the rId to track the maximum.
		numStr := strings.TrimPrefix(rel.ID, "rId")
		if n, err := strconv.Atoi(numStr); err == nil && n > maxID {
			maxID = n
		}
	}

	r.nextID = maxID + 1
	return r, nil
}

// Get returns the relationship with the given ID, or false.
func (r *Relations) Get(id string) (Relationship, bool) {
	rel, ok := r.rels[id]
	return rel, ok
}

// All returns all relationships.
func (r *Relations) All() []Relationship {
	out := make([]Relationship, 0, len(r.rels))
	for _, rel := range r.rels {
		out = append(out, rel)
	}
	return out
}

// Add adds a relationship with the given type, target, and target mode.
// Returns the assigned rId.
func (r *Relations) Add(relType, target, targetMode string) string {
	id := fmt.Sprintf("rId%d", r.nextID)
	r.nextID++
	r.rels[id] = Relationship{
		ID:         id,
		Type:       relType,
		Target:     target,
		TargetMode: targetMode,
	}
	return id
}

// AddImage adds an image relationship, returning the assigned rId.
// imagePath should be relative to the part (e.g. "media/image1.png").
func (r *Relations) AddImage(imagePath string) string {
	return r.Add(RelTypeImage, imagePath, "")
}

// AddHyperlink adds an external hyperlink relationship.
func (r *Relations) AddHyperlink(url string) string {
	return r.Add(RelTypeHyperlink, url, "External")
}

// Save writes the relations back to the Package as the .rels XML file.
func (r *Relations) Save(pkg *Package) error {
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)

	root := doc.CreateElement("Relationships")
	root.CreateAttr("xmlns", NSRelationships)

	for _, rel := range r.rels {
		elem := root.CreateElement("Relationship")
		elem.CreateAttr("Id", rel.ID)
		elem.CreateAttr("Type", rel.Type)
		elem.CreateAttr("Target", rel.Target)
		if rel.TargetMode != "" {
			elem.CreateAttr("TargetMode", rel.TargetMode)
		}
	}

	data, err := doc.WriteToBytes()
	if err != nil {
		return fmt.Errorf("docx: serialise relations for %q: %w", r.partURI, err)
	}

	pkg.SetPart(relsURI(r.partURI), data)
	return nil
}
