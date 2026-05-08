package docx

// NodeType classifies the kind of element a ViewNode represents.
type NodeType int

const (
	NodeHeading        NodeType = iota // Heading (Level 1-9)
	NodeParagraph                      // Normal paragraph
	NodeTable                          // Table container
	NodeImage                          // Inline or floating image
	NodeFootnote                       // Footnote reference
	NodeMath                           // Math equation (w:oMath / w:oMathPara)
	NodeListItem                       // Numbered or bulleted list item
	NodeTOC                            // Table of contents
	NodeBookmarkRef                    // Bookmark reference
	NodeContentControl                 // Structured document tag (w:sdt)
	NodeProtectedZone                  // Non-editable region
)

// String returns a human-readable label for the NodeType.
func (n NodeType) String() string {
	switch n {
	case NodeHeading:
		return "heading"
	case NodeParagraph:
		return "paragraph"
	case NodeTable:
		return "table"
	case NodeImage:
		return "image"
	case NodeFootnote:
		return "footnote"
	case NodeMath:
		return "math"
	case NodeListItem:
		return "list-item"
	case NodeTOC:
		return "toc"
	case NodeBookmarkRef:
		return "bookmark"
	case NodeContentControl:
		return "content-control"
	case NodeProtectedZone:
		return "protected"
	default:
		return "unknown"
	}
}

// ViewNode is a high-level representation of a document element.
// It is produced by the viewer from the raw OOXML DOM and consumed
// by tools (View, KBAdd) and renderers (Markdown, Terminal).
type ViewNode struct {
	Type     NodeType          // Element category
	Level    int               // Heading level (1-9) or list indent level
	Content  string            // Plain-text content
	Style    []string          // e.g. ["bold", "italic", "underline", "strike"]
	Children []ViewNode        // Table rows→cells, list sub-items, etc.
	Anchor   StableAnchor      // Stable locator for edits
	Metadata map[string]string // Image path, bookmark name, field codes, etc.
}

// StableAnchor uniquely identifies a position within the document,
// used for round-trip editing: View → Agent decision → Edit at anchor.
type StableAnchor struct {
	Part     string // e.g. "word/document.xml", "word/header1.xml"
	ParaID   string // w14:paraId (Word-assigned)
	Bookmark string // Bookmark name (optional)
	CCID     string // Content-control ID (optional)
	Offset   int    // Character offset within the paragraph
}

// ProtectedZone marks a document region that must not be modified.
type ProtectedZone struct {
	Name  string
	Start StableAnchor
	End   StableAnchor
}
