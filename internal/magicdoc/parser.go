package magicdoc

import (
	"strings"
	"time"
)

// Document 已识别的 MagicDoc
type Document struct {
	SessionID     string
	Path          string
	Title         string    // # MAGIC DOC: {title} 中的 title
	Instruction   string    // header 中的更新指令（MAGIC DOC 后面的内容行）
	LastReadAt    time.Time
	LastUpdatedAt time.Time
}

// ParseHeader 解析文件中的 MAGIC DOC header
// 格式: 文件中有一行 "# MAGIC DOC: <title>"（不区分大小写 MAGIC DOC 前缀）
// 后面可以跟几行指令（到第一个空行结束）
// 返回 Document + true，或 zero + false
func ParseHeader(content string) (Document, bool) {
	lines := strings.Split(content, "\n")

	magicLineIdx := -1
	var title string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		// Match "# MAGIC DOC: <title>" — prefix must be "# MAGIC DOC:"
		if strings.HasPrefix(upper, "# MAGIC DOC:") {
			// Extract title from original line preserving case
			colonIdx := strings.Index(strings.ToUpper(line), "# MAGIC DOC:")
			rest := line[colonIdx+len("# MAGIC DOC:"):]
			title = strings.TrimSpace(rest)
			magicLineIdx = i
			break
		}
	}

	if magicLineIdx < 0 {
		return Document{}, false
	}

	// Collect instruction lines: lines after magic line until empty line
	var instrLines []string
	for i := magicLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		instrLines = append(instrLines, line)
	}

	instruction := strings.Join(instrLines, "\n")

	doc := Document{
		Title:       title,
		Instruction: instruction,
	}
	return doc, true
}
