package docx

// OOXML namespace URIs
const (
	NSWordprocessingML = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"          // w:
	NSRelationships    = "http://schemas.openxmlformats.org/package/2006/relationships"           // r:
	NSContentTypes     = "http://schemas.openxmlformats.org/package/2006/content-types"
	NSDrawingML        = "http://schemas.openxmlformats.org/drawingml/2006/main"                  // a:
	NSDrawingWordML    = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" // wp:
	NSW14              = "http://schemas.microsoft.com/office/word/2010/wordml"                   // w14:
	NSW15              = "http://schemas.microsoft.com/office/word/2012/wordml"                   // w15:
	NSMCE              = "http://schemas.openxmlformats.org/markup-compatibility/2006"            // mc:
)

// Common element names with namespace prefixes
const (
	ElemBody          = "w:body"
	ElemParagraph     = "w:p"
	ElemRun           = "w:r"
	ElemText          = "w:t"
	ElemTable         = "w:tbl"
	ElemTableRow      = "w:tr"
	ElemTableCell     = "w:tc"
	ElemRunProps      = "w:rPr"
	ElemParaProps     = "w:pPr"
	ElemStyle         = "w:pStyle"
	ElemBookmarkStart = "w:bookmarkStart"
	ElemBookmarkEnd   = "w:bookmarkEnd"
	ElemSDT           = "w:sdt"
	ElemSDTAlias      = "w:alias"
	ElemSectPr        = "w:sectPr"
)
