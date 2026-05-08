package fileop

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

type TextReadResult struct {
	Lines      []string
	TotalLines int
	Encoding   string
	LineEnding string
	Content    string // content returned to the model, not necessarily the full file
	FullHash   string
	IsPartial  bool
}

func ReadTextRange(path string, offset, limit, maxLineChars int, maxSizeBytes int64) (TextReadResult, error) {
	st, err := os.Stat(path)
	if err != nil {
		return TextReadResult{}, err
	}
	fullHashEligible := maxSizeBytes <= 0 || st.Size() <= maxSizeBytes
	if limit <= 0 && !fullHashEligible {
		return TextReadResult{}, fmt.Errorf("file too large (%d bytes), use offset/limit", st.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return TextReadResult{}, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	hasher := sha256.New()
	res := TextReadResult{Encoding: "utf-8", LineEnding: "lf", IsPartial: limit > 0 || !fullHashEligible}
	lineNo := 0
	start := offset
	if start < 0 {
		start = 0
	}
	var content strings.Builder
	for {
		raw, readErr := reader.ReadString('\n')
		if raw != "" && fullHashEligible {
			_, _ = hasher.Write([]byte(raw))
		}
		if raw == "" && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return TextReadResult{}, readErr
		}

		lineNo++
		if lineNo == 1 && strings.HasPrefix(raw, "\uFEFF") {
			res.Encoding = "utf-8-bom"
			raw = strings.TrimPrefix(raw, "\uFEFF")
		}
		if strings.HasSuffix(raw, "\r\n") {
			res.LineEnding = "crlf"
		}
		line := strings.TrimSuffix(raw, "\n")
		line = strings.TrimSuffix(line, "\r")
		if maxLineChars > 0 && len(line) > maxLineChars {
			line = line[:maxLineChars] + "..."
		}
		if lineNo <= start {
			if readErr == io.EOF {
				break
			}
			continue
		}
		if limit <= 0 || len(res.Lines) < limit {
			res.Lines = append(res.Lines, line)
			if content.Len() > 0 {
				content.WriteByte('\n')
			}
			content.WriteString(line)
		}
		if !fullHashEligible && limit > 0 && len(res.Lines) >= limit {
			break
		}
		if readErr == io.EOF {
			break
		}
	}
	res.TotalLines = lineNo
	res.Content = content.String()
	if fullHashEligible {
		res.FullHash = hex.EncodeToString(hasher.Sum(nil))
	}
	return res, nil
}
