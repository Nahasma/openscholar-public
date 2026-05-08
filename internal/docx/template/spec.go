package template

import "github.com/Nahasma/openscholar-public/internal/docx"

// AnchorType ranks the locating mechanism by reliability.
type AnchorType int

const (
	AnchorContentControl AnchorType = iota // Highest priority
	AnchorBookmark                          // Medium
	AnchorPlaceholder                       // Fallback
)

// SlotDataType describes what kind of data a slot accepts.
type SlotDataType int

const (
	SlotText        SlotDataType = iota // Plain string
	SlotRichText                        // Multi-paragraph
	SlotImage                           // ImageData
	SlotTable                           // []map[string]any (tabular rows)
	SlotConditional                     // bool
	SlotLoop                            // []map[string]any (repeating block)
)

// SlotSpec describes one fillable slot in a template.
type SlotSpec struct {
	Name       string            // Slot name (placeholder key / CC alias / bookmark name)
	AnchorType AnchorType        // How this slot was detected
	DataType   SlotDataType      // Expected data type
	Required   bool              // Whether omission is an error
	Default    string            // Default value if not supplied
	Anchor     docx.StableAnchor // Document position
}

// ProtectedZone is an alias for docx.ProtectedZone.
type ProtectedZone = docx.ProtectedZone

// SectionSpec describes a document section (heading + body).
type SectionSpec struct {
	Title    string
	Level    int
	Anchor   docx.StableAnchor
	Children []SectionSpec
}

// TemplateSpec is the full analysis result for a template document.
type TemplateSpec struct {
	Slots          []SlotSpec
	ProtectedZones []ProtectedZone
	Sections       []SectionSpec
	Metadata       map[string]string
}

// ImageData holds the data needed to insert an image.
type ImageData struct {
	Path   string // Local file path to the image
	Width  int    // EMU width (1 inch = 914400 EMU), 0 = auto
	Height int    // EMU height, 0 = auto
}
