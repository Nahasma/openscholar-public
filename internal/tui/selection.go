package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/openscholar/openscholar/internal/tui/components"
)

var selectionStyle = lipgloss.NewStyle().Reverse(true)

// screenToContentPos converts screen coordinates to content line/column indices.
// Returns (-1, -1) if the position is outside the viewport area.
func (m *Model) screenToContentPos(screenX, screenY int) (row, col int) {
	row, col, _, ok := m.screenToSelectionPos(screenX, screenY)
	if !ok {
		return -1, -1
	}
	return row, col
}

func (m *Model) screenToSelectionPos(screenX, screenY int) (row, col int, coord TranscriptCoord, ok bool) {
	localY := screenY - m.headerLayoutHeight()
	if localY < 0 || localY >= m.chat.viewport.Height {
		return -1, -1, TranscriptCoord{}, false
	}
	row = m.chat.viewport.YOffset + localY
	col = screenX
	if col < 0 {
		col = 0
	}
	coord = m.transcriptCoordForContentRow(row, col)
	return row, col, coord, true
}

func (m *Model) transcriptCoordForContentRow(contentRow, col int) TranscriptCoord {
	if col < 0 {
		col = 0
	}
	absLine := contentRow
	defaultH := 1
	if m.usesVirtualTranscript() {
		absLine = m.chat.virtualList.LastFrameTopLine() + contentRow
		defaultH = components.DefaultBlockHeight
	}
	coord := TranscriptCoord{
		ContentRow:  contentRow,
		AbsLine:     absLine,
		Column:      col,
		PlainOffset: -1,
		BlockIdx:    -1,
	}
	if m.chat.blockList == nil || m.chat.heightCache == nil || m.chat.blockList.Len() == 0 {
		return coord
	}
	blocks := m.chat.blockList.All()
	if !m.contentRowInBlockTranscript(contentRow, absLine, blocks, defaultH) {
		return coord
	}
	anchor := components.AnchorFromDisplayLine(m.chat.heightCache, blocks, absLine, defaultH)
	if anchor.BoundaryBefore {
		return coord
	}
	coord.BlockIdx = anchor.BlockIdx
	coord.LineInBlock = anchor.LineOffset
	coord.BoundaryBefore = anchor.BoundaryBefore
	if block := m.chat.blockList.Get(anchor.BlockIdx); block != nil {
		coord.Valid = true
		coord.BlockID = block.ID
		coord.MsgID = block.MsgID
		coord.PlainOffset, coord.LineStart = flattenedRenderedPosition(block.Rendered, coord.LineInBlock, col)
	}
	return coord
}

func (m *Model) contentRowInBlockTranscript(contentRow, absLine int, blocks []components.BlockVM, defaultH int) bool {
	if contentRow < 0 || absLine < 0 {
		return false
	}
	if m.usesVirtualTranscript() {
		if m.chat.virtualList == nil {
			return false
		}
		return contentRow < m.chat.virtualList.LastFrameHeight()
	}
	return absLine < components.DisplayTotalHeight(m.chat.heightCache, blocks, defaultH)
}

// normalizeSelection ensures start is before end (supports reverse drag).
func normalizeSelection(sel TextSelection) (startRow, startCol, endRow, endCol int) {
	startRow, startCol = sel.StartRow, sel.StartCol
	endRow, endCol = sel.EndRow, sel.EndCol
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, startCol, endRow, endCol = endRow, endCol, startRow, startCol
	}
	return
}

func (m *Model) syncSelectionRowsFromSemanticCoords() {
	if !m.chat.selection.Active && !m.chat.selection.HasRange {
		return
	}
	m.reanchorSelectionText()
	if m.chat.selection.StartCoord.Valid {
		coord := m.resolveTranscriptCoord(m.chat.selection.StartCoord)
		m.chat.selection.StartCoord = coord
		m.chat.selection.StartRow = coord.ContentRow
		m.chat.selection.StartCol = coord.Column
	}
	if m.chat.selection.EndCoord.Valid {
		coord := m.resolveTranscriptCoord(m.chat.selection.EndCoord)
		m.chat.selection.EndCoord = coord
		m.chat.selection.EndRow = coord.ContentRow
		m.chat.selection.EndCol = coord.Column
	}
}

func (m *Model) reanchorSelectionText() {
	text := m.chat.selection.PlainText
	if text == "" || m.chat.blockList == nil {
		return
	}
	anchorText := selectionAnchorText(text)
	if anchorText == "" {
		return
	}
	trailingLineStart := selectionEndsWithLineBreak(text)
	start := m.chat.selection.StartCoord
	end := m.chat.selection.EndCoord
	if !start.Valid || !end.Valid || start.BlockID == "" || start.BlockID != end.BlockID {
		return
	}
	block := m.chat.blockList.GetByID(start.BlockID)
	if block == nil {
		return
	}
	flat := flattenedRenderedText(block.Rendered)
	preferred := start.PlainOffset
	if end.PlainOffset >= 0 && (preferred < 0 || end.PlainOffset < preferred) {
		preferred = end.PlainOffset
	}
	left, ok := nearestTextOffset(flat, anchorText, preferred)
	if !ok {
		return
	}
	textLen := len([]rune(anchorText))
	if start.PlainOffset >= 0 && end.PlainOffset >= 0 && start.PlainOffset > end.PlainOffset {
		start.PlainOffset = left + textLen
		start.LineStart = trailingLineStart
		end.PlainOffset = left
		end.LineStart = false
	} else {
		start.PlainOffset = left
		start.LineStart = false
		end.PlainOffset = left + textLen
		end.LineStart = trailingLineStart
	}
	m.chat.selection.StartCoord = start
	m.chat.selection.EndCoord = end
}

func nearestTextOffset(flat, text string, preferred int) (int, bool) {
	if flat == "" || text == "" {
		return 0, false
	}
	flatRunes := []rune(flat)
	textRunes := []rune(text)
	if preferred >= 0 && preferred+len(textRunes) <= len(flatRunes) &&
		string(flatRunes[preferred:preferred+len(textRunes)]) == text {
		return preferred, true
	}

	best := -1
	bestDistance := 0
	searchFrom := 0
	for {
		idx := strings.Index(flat[searchFrom:], text)
		if idx < 0 {
			break
		}
		byteOffset := searchFrom + idx
		runeOffset := len([]rune(flat[:byteOffset]))
		distance := absInt(runeOffset - preferred)
		if best < 0 || distance < bestDistance {
			best = runeOffset
			bestDistance = distance
		}
		searchFrom = byteOffset + len(text)
	}
	return best, best >= 0
}

func selectionAnchorText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "")
	text = strings.ReplaceAll(text, "\r", "")
	return text
}

func selectionEndsWithLineBreak(text string) bool {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\r")
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *Model) resolveTranscriptCoord(coord TranscriptCoord) TranscriptCoord {
	if !coord.Valid {
		return coord
	}
	absLine := coord.AbsLine
	defaultH := 1
	if m.usesVirtualTranscript() {
		defaultH = components.DefaultBlockHeight
	}
	if m.chat.blockList != nil && m.chat.heightCache != nil && m.chat.blockList.Len() > 0 {
		anchor := components.ScrollAnchor{
			BlockID:        coord.BlockID,
			MsgID:          coord.MsgID,
			BlockIdx:       coord.BlockIdx,
			LineOffset:     coord.LineInBlock,
			BoundaryBefore: coord.BoundaryBefore,
		}
		normalized := anchor.Normalize(m.chat.heightCache, m.chat.blockList.All(), defaultH)
		coord.BlockIdx = normalized.BlockIdx
		coord.LineInBlock = normalized.LineOffset
		coord.BoundaryBefore = normalized.BoundaryBefore
		if block := m.chat.blockList.Get(coord.BlockIdx); block != nil {
			coord.BlockID = block.ID
			coord.MsgID = block.MsgID
			if coord.PlainOffset >= 0 && !coord.BoundaryBefore {
				coord.LineInBlock, coord.Column = renderedLineColumnForOffset(block.Rendered, coord.PlainOffset, coord.LineStart)
			}
		}
		normalized.LineOffset = coord.LineInBlock
		absLine = normalized.ResolveDisplayLine(m.chat.heightCache, m.chat.blockList.All(), defaultH)
	}
	coord.AbsLine = absLine
	coord.ContentRow = absLine
	if m.usesVirtualTranscript() {
		coord.ContentRow = absLine - m.chat.virtualList.LastFrameTopLine()
	}
	return coord
}

func flattenedRenderedPosition(rendered string, lineIdx, col int) (int, bool) {
	if rendered == "" {
		return -1, false
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	offset := 0
	for i := 0; i < lineIdx; i++ {
		text, _ := selectionRenderedLineText(lines, i)
		offset += len([]rune(text))
	}
	text, prefix := selectionRenderedLineText(lines, lineIdx)
	runes := []rune(text)
	col -= prefix
	if col < 0 {
		col = 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	return offset + col, lineIdx > 0 && col == 0
}

func renderedLineColumnForOffset(rendered string, offset int, lineStart bool) (lineIdx, col int) {
	if offset < 0 || rendered == "" {
		return 0, 0
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	for i := range lines {
		text, prefix := selectionRenderedLineText(lines, i)
		lineLen := len([]rune(text))
		if offset < lineLen || (offset == lineLen && (!lineStart || i == len(lines)-1)) {
			return i, offset + prefix
		}
		if offset == lineLen && lineStart && i < len(lines)-1 {
			_, nextPrefix := selectionRenderedLineText(lines, i+1)
			return i + 1, nextPrefix
		}
		offset -= lineLen
	}
	last := len(lines) - 1
	text, prefix := selectionRenderedLineText(lines, last)
	return last, len([]rune(text)) + prefix
}

func flattenedRenderedText(rendered string) string {
	if rendered == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	var sb strings.Builder
	for i := range lines {
		text, _ := selectionRenderedLineText(lines, i)
		sb.WriteString(text)
	}
	return sb.String()
}

func renderedTextRange(rendered string, left, right int, includeTrailingLineBreak bool) string {
	if rendered == "" || left > right {
		return ""
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	var sb strings.Builder
	offset := 0
	wrote := false
	for i := range lines {
		text, _ := selectionRenderedLineText(lines, i)
		runes := []rune(text)
		lineStart := offset
		lineEnd := lineStart + len(runes)
		offset = lineEnd
		if right <= lineStart {
			break
		}
		if left >= lineEnd {
			continue
		}
		start := max(0, left-lineStart)
		end := min(len(runes), right-lineStart)
		if start > end {
			start = end
		}
		if wrote {
			sb.WriteString("\n")
		}
		sb.WriteString(string(runes[start:end]))
		wrote = true
	}
	if includeTrailingLineBreak {
		sb.WriteString("\n")
	}
	return sb.String()
}

func selectionLineText(line string) (string, int) {
	return selectionLineTextWithContext(nil, 0, line)
}

func selectionRenderedLineText(lines []string, idx int) (string, int) {
	if idx < 0 || idx >= len(lines) {
		return "", 0
	}
	return selectionLineTextWithContext(lines, idx, lines[idx])
}

func selectionLineTextWithContext(lines []string, idx int, line string) (string, int) {
	plain := xansi.Strip(line)
	for _, prefix := range transcriptMarkerPrefixes() {
		if strings.HasPrefix(plain, prefix) {
			return plain[len(prefix):], len([]rune(prefix))
		}
	}
	if idx > 0 && len(lines) > 0 {
		if renderedLinesStartWith(lines, "  "+components.FigBlackCircle+" ") {
			prefix := strings.Repeat(" ", components.MessagePrefixWidth())
			if strings.HasPrefix(plain, prefix) {
				return plain[len(prefix):], len([]rune(prefix))
			}
		}
		if renderedLinesStartWith(lines, "  "+components.FigConnector+"  ") {
			prefix := strings.Repeat(" ", components.ResponsePrefixWidth())
			if strings.HasPrefix(plain, prefix) {
				return plain[len(prefix):], len([]rune(prefix))
			}
		}
	}
	if strings.HasPrefix(plain, "  ") {
		return plain[2:], 2
	}
	return plain, 0
}

func renderedLinesStartWith(lines []string, prefix string) bool {
	if len(lines) == 0 {
		return false
	}
	return strings.HasPrefix(xansi.Strip(lines[0]), prefix)
}

func transcriptMarkerPrefixes() []string {
	return []string{
		"  " + components.FigConnector + "  ",
		"  " + components.FigBlackCircle + " ",
		"  " + components.FigTreeMid + " ",
		"  " + components.FigTreeEnd + " ",
		"  " + components.FigTreeVert + "  ",
	}
}

func orderedTranscriptCoords(a, b TranscriptCoord) (TranscriptCoord, TranscriptCoord) {
	if compareTranscriptCoord(a, b) > 0 {
		return b, a
	}
	return a, b
}

func compareTranscriptCoord(a, b TranscriptCoord) int {
	if a.PlainOffset < b.PlainOffset {
		return -1
	}
	if a.PlainOffset > b.PlainOffset {
		return 1
	}
	if a.LineStart == b.LineStart {
		return 0
	}
	if a.LineStart {
		return 1
	}
	return -1
}

// extractSelectedText extracts plain text from the selected range.
func (m *Model) extractSelectedText() string {
	m.syncSelectionRowsFromSemanticCoords()
	if text, ok := m.extractSelectedTextFromSemanticCoords(); ok {
		m.chat.selection.PlainText = text
		return text
	}
	if len(m.chat.contentLines) == 0 {
		return ""
	}

	startRow, startCol, endRow, endCol := normalizeSelection(m.chat.selection)

	// Clamp to valid range
	if startRow < 0 {
		startRow = 0
	}
	if endRow >= len(m.chat.contentLines) {
		endRow = len(m.chat.contentLines) - 1
	}
	if startRow > endRow {
		return ""
	}

	var sb strings.Builder
	for row := startRow; row <= endRow; row++ {
		line := m.chat.contentLines[row]
		plainLine := xansi.Strip(line)
		lineWidth := len([]rune(plainLine))

		var left, right int
		if row == startRow {
			left = startCol
		}
		if row == endRow {
			right = endCol
		} else {
			right = lineWidth
		}

		// Clamp
		if left < 0 {
			left = 0
		}
		if right > lineWidth {
			right = lineWidth
		}
		if left > right {
			left = right
		}

		runes := []rune(plainLine)
		if left < len(runes) {
			end := right
			if end > len(runes) {
				end = len(runes)
			}
			sb.WriteString(string(runes[left:end]))
		}

		if row < endRow {
			sb.WriteString("\n")
		}
	}
	text := sb.String()
	m.chat.selection.PlainText = text
	return text
}

func (m *Model) extractSelectedTextFromSemanticCoords() (string, bool) {
	start := m.chat.selection.StartCoord
	end := m.chat.selection.EndCoord
	if !start.Valid || !end.Valid || start.BlockID == "" || start.BlockID != end.BlockID {
		return "", false
	}
	if start.PlainOffset < 0 || end.PlainOffset < 0 || m.chat.blockList == nil {
		return "", false
	}
	block := m.chat.blockList.GetByID(start.BlockID)
	if block == nil {
		return "", false
	}
	if compareTranscriptCoord(start, end) == 0 {
		return "", false
	}
	leftCoord, rightCoord := orderedTranscriptCoords(start, end)
	left, right := leftCoord.PlainOffset, rightCoord.PlainOffset
	flatRunes := []rune(flattenedRenderedText(block.Rendered))
	if left < 0 {
		left = 0
	}
	if right > len(flatRunes) {
		right = len(flatRunes)
	}
	if left >= right {
		if !rightCoord.LineStart || left != right {
			return "", false
		}
	}
	return renderedTextRange(block.Rendered, left, right, rightCoord.LineStart), true
}

// applySelectionHighlight applies reverse-video highlighting to selected lines.
func applySelectionHighlight(lines []string, sel TextSelection) []string {
	if !sel.Active && !sel.HasRange {
		return lines
	}

	startRow, startCol, endRow, endCol := normalizeSelection(sel)

	// Clamp
	if startRow < 0 {
		startRow = 0
	}
	if endRow >= len(lines) {
		endRow = len(lines) - 1
	}
	if startRow > endRow || (startRow == endRow && startCol == endCol) {
		return lines
	}

	result := make([]string, len(lines))
	copy(result, lines)

	for row := startRow; row <= endRow; row++ {
		line := lines[row]
		lineWidth := xansi.StringWidth(line)

		var left, right int
		if row == startRow {
			left = startCol
		}
		if row == endRow {
			right = endCol
		} else {
			right = lineWidth
		}

		if left < 0 {
			left = 0
		}
		if right > lineWidth {
			right = lineWidth
		}
		if left >= right {
			continue
		}

		// Split line into three segments: before, selected, after
		before := ""
		if left > 0 {
			before = xansi.Cut(line, 0, left)
		}
		selected := xansi.Cut(line, left, right)
		after := ""
		if right < lineWidth {
			after = xansi.Cut(line, right, lineWidth)
		}

		// Apply reverse video to selected segment
		highlighted := selectionStyle.Render(xansi.Strip(selected))
		result[row] = before + highlighted + after
	}

	return result
}

// clearSelection clears the current text selection.
func (m *Model) clearSelection() {
	if m.chat.selection.Active || m.chat.selection.HasRange {
		m.chat.selection = TextSelection{}
		_ = m.updateViewportContent()
	}
}
