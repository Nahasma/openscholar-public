package components

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	chromaFormatter chroma.Formatter
	chromaStyle     *chroma.Style
)

func init() {
	chromaFormatter = formatters.TTY256
	if IsDarkTheme {
		chromaStyle = styles.Get("monokai")
	} else {
		chromaStyle = styles.Get("github")
	}
	if chromaStyle == nil {
		chromaStyle = styles.Fallback
	}
}

// HighlightCode applies syntax highlighting to code using chroma.
// Returns the highlighted string, or the original code on error.
func HighlightCode(code, lang string, width int) string {
	if lang == "" || code == "" || chromaFormatter == nil {
		return code
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		return code
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf bytes.Buffer
	err = chromaFormatter.Format(&buf, chromaStyle, iterator)
	if err != nil {
		return code
	}

	// Remove trailing newline that chroma may add
	result := buf.String()
	result = strings.TrimRight(result, "\n")

	return result
}
