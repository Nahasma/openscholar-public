package docx

import (
	"strings"

	"github.com/beevik/etree"
)

// RunInfo describes a single w:r element's text content and style.
type RunInfo struct {
	Text      string
	Start     int    // character start position within the paragraph
	End       int    // character end position
	Bold      bool
	Italic    bool
	Underline bool
	Strike    bool
	FontName  string
	FontSize  string // e.g. "24" (half-points)
}

// RenderParagraph extracts the concatenated plain text and collected
// style flags from a w:p element.
func RenderParagraph(para *etree.Element) (text string, styles []string) {
	runs := RenderRuns(para)
	var sb strings.Builder
	seen := make(map[string]bool)
	for _, r := range runs {
		sb.WriteString(r.Text)
		if r.Bold && !seen["bold"] {
			styles = append(styles, "bold")
			seen["bold"] = true
		}
		if r.Italic && !seen["italic"] {
			styles = append(styles, "italic")
			seen["italic"] = true
		}
		if r.Underline && !seen["underline"] {
			styles = append(styles, "underline")
			seen["underline"] = true
		}
		if r.Strike && !seen["strike"] {
			styles = append(styles, "strike")
			seen["strike"] = true
		}
	}
	return sb.String(), styles
}

// RenderRuns extracts all Run elements from a paragraph with their
// character positions and style information.
func RenderRuns(para *etree.Element) []RunInfo {
	var runs []RunInfo
	pos := 0

	for _, child := range para.ChildElements() {
		if child.Tag != "r" {
			continue
		}

		var info RunInfo
		info.Start = pos

		// Extract run properties from w:rPr
		rpr := child.FindElement("rPr")
		if rpr != nil {
			for _, rp := range rpr.ChildElements() {
				switch rp.Tag {
				case "b":
					val := rp.SelectAttrValue("val", "")
					if val != "0" && val != "false" {
						info.Bold = true
					}
				case "i":
					val := rp.SelectAttrValue("val", "")
					if val != "0" && val != "false" {
						info.Italic = true
					}
				case "u":
					val := rp.SelectAttrValue("val", "")
					if val != "0" && val != "false" && val != "none" {
						info.Underline = true
					} else if val == "" {
						// presence of <w:u> without val="none" means underline
						info.Underline = true
					}
				case "strike":
					val := rp.SelectAttrValue("val", "")
					if val != "0" && val != "false" {
						info.Strike = true
					}
				case "rFonts":
					// Try "ascii" attribute (etree strips namespace prefix from key)
					font := rp.SelectAttrValue("ascii", "")
					if font == "" {
						// Some serialisers write it as a prefixed attr; etree
						// stores the Space separately but SelectAttr searches by key only.
						for _, attr := range rp.Attr {
							if attr.Key == "ascii" {
								font = attr.Value
								break
							}
						}
					}
					info.FontName = font
				case "sz":
					sz := rp.SelectAttrValue("val", "")
					if sz == "" {
						for _, attr := range rp.Attr {
							if attr.Key == "val" {
								sz = attr.Value
								break
							}
						}
					}
					info.FontSize = sz
				}
			}
		}

		// Collect text from all w:t children
		var sb strings.Builder
		for _, t := range child.ChildElements() {
			if t.Tag == "t" {
				sb.WriteString(t.Text())
			}
		}
		info.Text = sb.String()
		pos += len([]rune(info.Text))
		info.End = pos

		runs = append(runs, info)
	}
	return runs
}
