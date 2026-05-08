package components

import "strings"

type NoticeKind uint8

const (
	NoticeInfo NoticeKind = iota
	NoticeSuccess
	NoticeWarning
	NoticeError
)

type TransientNotice struct {
	Text string
	Kind NoticeKind
}

func (n TransientNotice) IsZero() bool {
	return strings.TrimSpace(n.Text) == ""
}
