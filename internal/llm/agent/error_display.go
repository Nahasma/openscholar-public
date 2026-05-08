package agent

import (
	"errors"
	"strings"
)

type runtimeDisplayError interface {
	UserMessage() string
	Detail() string
}

func ErrorDisplay(err error, _ TerminalReason) (summary string, detail string) {
	if err == nil {
		return "", ""
	}
	var de runtimeDisplayError
	if errors.As(err, &de) {
		summary = strings.TrimSpace(de.UserMessage())
		detail = strings.TrimSpace(de.Detail())
		if summary != "" && detail != "" {
			return summary, detail
		}
	}
	return "Request failed. Press Ctrl+O for details.", strings.TrimSpace(err.Error())
}
