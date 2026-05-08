package docx

import (
	"archive/zip"
	"fmt"
	"io"
	"sort"

	"github.com/beevik/etree"
	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// Package represents an OOXML document package (docx/dotx).
type Package struct {
	path      string           // original file path
	parts     map[string]*Part // URI → Part
	modified  map[string]bool  // marks modified Parts
	zipReader *zip.ReadCloser  // holds the original ZIP reader open
}

// Part represents a single file inside the ZIP (XML or binary).
type Part struct {
	URI         string          // e.g. "word/document.xml"
	ContentType string          // e.g. "application/xml"
	Data        []byte          // raw bytes (kept as-is when unmodified)
	Doc         *etree.Document // populated on demand for XML parts
	rawFile     *zip.File       // reference to original zip.File for zero-copy Save
}

// Open opens a docx/dotx file and reads all entries into memory.
func Open(path string) (*Package, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("docx: open zip %q: %w", path, err)
	}

	pkg := &Package{
		path:      path,
		parts:     make(map[string]*Part, len(zr.File)),
		modified:  make(map[string]bool),
		zipReader: zr,
	}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			_ = zr.Close()
			return nil, fmt.Errorf("docx: open entry %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			_ = zr.Close()
			return nil, fmt.Errorf("docx: read entry %q: %w", f.Name, err)
		}

		pkg.parts[f.Name] = &Part{
			URI:     f.Name,
			Data:    data,
			rawFile: f,
		}
	}

	return pkg, nil
}

// Save writes the package to path. Unmodified parts are zero-copy copied;
// modified parts are re-compressed with their new Data.
func (p *Package) Save(path string) error {
	out, err := fileop.CreateFileAtomic(path, 0o644)
	if err != nil {
		return fmt.Errorf("docx: create %q: %w", path, err)
	}

	zw := zip.NewWriter(out)

	// Iterate in a deterministic order.
	uris := p.Parts()
	sort.Strings(uris)

	for _, uri := range uris {
		part := p.parts[uri]

		if !p.modified[uri] && part.rawFile != nil {
			// Zero-copy: transparently copy compressed data from the original ZIP.
			if err := zw.Copy(part.rawFile); err != nil {
				_ = zw.Close()
				_ = out.Abort()
				return fmt.Errorf("docx: copy part %q: %w", uri, err)
			}
			continue
		}

		// Modified (or newly added) part: write with a fresh header.
		var fh zip.FileHeader
		if part.rawFile != nil {
			fh = part.rawFile.FileHeader
		} else {
			fh = zip.FileHeader{
				Name:   uri,
				Method: zip.Deflate,
			}
		}
		// Reset compressed-size field so zip.Writer recalculates it.
		fh.CompressedSize64 = 0

		w, err := zw.CreateHeader(&fh)
		if err != nil {
			_ = zw.Close()
			_ = out.Abort()
			return fmt.Errorf("docx: create header %q: %w", uri, err)
		}
		if _, err := w.Write(part.Data); err != nil {
			_ = zw.Close()
			_ = out.Abort()
			return fmt.Errorf("docx: write part %q: %w", uri, err)
		}
	}

	if err := zw.Close(); err != nil {
		_ = out.Abort()
		return fmt.Errorf("docx: close zip writer: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("docx: close output file: %w", err)
	}
	return nil
}

// Part returns the Part for the given URI, or an error if not found.
func (p *Package) Part(uri string) (*Part, error) {
	part, ok := p.parts[uri]
	if !ok {
		return nil, fmt.Errorf("docx: part %q not found", uri)
	}
	return part, nil
}

// SetPart updates (or creates) a Part's raw bytes and marks it modified.
func (p *Package) SetPart(uri string, data []byte) {
	part, ok := p.parts[uri]
	if !ok {
		part = &Part{URI: uri}
		p.parts[uri] = part
	}
	part.Data = data
	part.Doc = nil // invalidate cached XML DOM
	p.modified[uri] = true
}

// XML returns the parsed etree.Document for the Part at uri.
// The result is cached on the Part for subsequent calls.
func (p *Package) XML(uri string) (*etree.Document, error) {
	part, err := p.Part(uri)
	if err != nil {
		return nil, err
	}
	if part.Doc != nil {
		return part.Doc, nil
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(part.Data); err != nil {
		return nil, fmt.Errorf("docx: parse XML for %q: %w", uri, err)
	}
	part.Doc = doc
	return doc, nil
}

// SetXML serialises doc and updates the Part at uri.
func (p *Package) SetXML(uri string, doc *etree.Document) error {
	doc.WriteSettings = etree.WriteSettings{
		CanonicalEndTags: false,
		CanonicalText:    false,
		CanonicalAttrVal: false,
	}
	data, err := doc.WriteToBytes()
	if err != nil {
		return fmt.Errorf("docx: serialise XML for %q: %w", uri, err)
	}
	p.SetPart(uri, data)
	// Keep Doc in sync so callers that still hold the pointer see the latest.
	p.parts[uri].Doc = doc
	return nil
}

// Close releases the underlying ZIP reader.
func (p *Package) Close() error {
	if p.zipReader != nil {
		return p.zipReader.Close()
	}
	return nil
}

// Parts returns the URIs of all parts in the package, in no particular order.
func (p *Package) Parts() []string {
	uris := make([]string, 0, len(p.parts))
	for uri := range p.parts {
		uris = append(uris, uri)
	}
	return uris
}

// HasPart reports whether a Part with the given URI exists.
func (p *Package) HasPart(uri string) bool {
	_, ok := p.parts[uri]
	return ok
}
