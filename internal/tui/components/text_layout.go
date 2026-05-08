package components

// text_layout.go — 统一的 TUI 文本布局工具层
//
// 所有面向终端渲染的截断、换行、宽度计算都应通过本文件的函数执行。
// 禁止在 TUI 组件中直接使用 s[:N] 或 len(s) 做布局计算。

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// displayWidth 返回字符串的终端显示宽度。
// CJK 字符算 2 列，ANSI 转义序列算 0 列。
func displayWidth(s string) int {
	return lipgloss.Width(s)
}

// truncateDisplay 按显示宽度截断，超出部分替换为 …
// ANSI-safe，通过 xansi.Truncate 实现。
func truncateDisplay(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	return xansi.Truncate(s, maxWidth, "…")
}

// truncateDisplayStart 从头部截断，保留尾部可见内容，前置 …
// 用于输入框"光标在末尾"时只显示尾部文本。
func truncateDisplayStart(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	runes := []rune(s)
	w := 0
	startIdx := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if w+rw > maxWidth-1 { // -1 for …
			break
		}
		w += rw
		startIdx = i
	}
	return "…" + string(runes[startIdx:])
}

// truncateDisplayNoTail 按显示宽度截断，不追加省略号。
// 用于调用方自行拼接分隔符的场景（如路径中间截断）。
func truncateDisplayNoTail(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	w := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth {
			return s[:i]
		}
		w += rw
	}
	return s
}

// truncatePathMiddle 路径中间截断，优先保留文件名。
// "src/deeply/nested/dir/file.go" → "src/dee…/file.go"
func truncatePathMiddle(path string, maxWidth int) string {
	if runewidth.StringWidth(path) <= maxWidth {
		return path
	}
	if maxWidth <= 5 {
		return truncateDisplay(path, maxWidth)
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		return truncateDisplay(path, maxWidth)
	}
	filename := path[lastSlash:] // 含前导 /
	dir := path[:lastSlash]
	fnWidth := runewidth.StringWidth(filename)
	if fnWidth >= maxWidth-1 {
		return truncateDisplayStart(path, maxWidth)
	}
	availForDir := maxWidth - 1 - fnWidth // -1 for …
	if availForDir <= 0 {
		return truncateDisplayStart(filename, maxWidth)
	}
	truncDir := truncateDisplayNoTail(dir, availForDir)
	return truncDir + "…" + filename
}

// wrapDisplay 按显示宽度换行（CJK-aware），返回行切片。
func wrapDisplay(s string, maxWidth int) []string {
	if maxWidth <= 0 || runewidth.StringWidth(s) <= maxWidth {
		return []string{s}
	}
	var result []string
	var current strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth && current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
			w = 0
		}
		current.WriteRune(r)
		w += rw
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

// wrapDisplayString 按显示宽度换行，返回以 \n 拼接的字符串。
func wrapDisplayString(s string, maxWidth int) string {
	if maxWidth <= 0 || runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	var sb strings.Builder
	w := 0
	first := true
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth && !first {
			sb.WriteByte('\n')
			w = 0
			first = true
		}
		sb.WriteRune(r)
		w += rw
		first = false
	}
	return sb.String()
}
