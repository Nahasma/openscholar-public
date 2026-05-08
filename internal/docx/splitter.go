package docx

import (
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

// PlaceholderType classifies the kind of placeholder.
type PlaceholderType int

const (
	PTVariable   PlaceholderType = iota // {{key}}
	PTLoopStart                         // {{#key}}
	PTLoopEnd                           // {{/key}}
	PTCondStart                         // {{?key}}
	PTCondEnd                           // same syntax as loop end, disambiguated by context
	PTImage                             // {{img:key}}
	PTProtected                         // {{!key}}
	PTExpression                        // {{key | filter}}
)

// PlaceholderMatch describes one found placeholder in a paragraph,
// including which Runs it spans and exact character offsets.
type PlaceholderMatch struct {
	FullText    string           // e.g. "{{title}}"
	Key         string           // e.g. "title"
	Type        PlaceholderType
	Runs        []*etree.Element // the Run elements this placeholder spans
	StartRun    int              // index of start Run in paragraph
	EndRun      int              // index of end Run
	StartOffset int              // char offset within StartRun's text (rune-based)
	EndOffset   int              // char offset within EndRun's text (rune-based), exclusive
}

// PlaceholderSyntax configures the delimiters used for placeholder matching.
type PlaceholderSyntax struct {
	Open  string // default "{{"
	Close string // default "}}"
}

// DefaultSyntax returns the standard {{ }} placeholder syntax.
func DefaultSyntax() PlaceholderSyntax {
	return PlaceholderSyntax{Open: "{{", Close: "}}"}
}

// charPos records a character's position back to a specific run and intra-run offset.
type charPos struct {
	runIndex int
	offset   int // rune offset within the run text
}

// runText extracts the concatenated w:t text from a w:r element.
func runText(r *etree.Element) string {
	var sb strings.Builder
	for _, child := range r.ChildElements() {
		if child.Tag == "t" {
			sb.WriteString(child.Text())
		}
	}
	return sb.String()
}

// placeholderRegex matches a placeholder of the form {{...}} where the inner
// part may contain any character except closing }}.
// We match the outermost {{ ... }} that does not itself contain {{ to avoid
// "nesting" complications (inner match wins naturally in scan order).
var placeholderRegex = regexp.MustCompile(`\{\{[^{]*?\}\}`)

// classifyPlaceholder parses the inner key of a matched placeholder and returns
// the PlaceholderType and cleaned Key string.
func classifyPlaceholder(inner string) (PlaceholderType, string) {
	if strings.HasPrefix(inner, "#") {
		return PTLoopStart, inner[1:]
	}
	if strings.HasPrefix(inner, "/") {
		return PTLoopEnd, inner[1:]
	}
	if strings.HasPrefix(inner, "?") {
		return PTCondStart, inner[1:]
	}
	if strings.HasPrefix(inner, "img:") {
		return PTImage, inner[4:]
	}
	if strings.HasPrefix(inner, "!") {
		return PTProtected, inner[1:]
	}
	if strings.Contains(inner, "|") {
		return PTExpression, inner
	}
	return PTVariable, inner
}

// FindPlaceholders scans a w:p element for placeholders, handling the case
// where a single placeholder's text is split across multiple w:r elements.
func FindPlaceholders(para *etree.Element, syntax PlaceholderSyntax) []PlaceholderMatch {
	// Collect all direct w:r children (runs) in order.
	var runs []*etree.Element
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			runs = append(runs, child)
		}
	}
	if len(runs) == 0 {
		return nil
	}

	// Build concatenated string and a charMap: charMap[globalRuneIndex] = charPos.
	var sb strings.Builder
	var charMap []charPos

	for ri, r := range runs {
		text := runText(r)
		runes := []rune(text)
		for oi := range runes {
			charMap = append(charMap, charPos{runIndex: ri, offset: oi})
			sb.WriteRune(runes[oi])
		}
	}

	full := sb.String()
	if full == "" {
		return nil
	}

	// Build a pattern based on the syntax delimiters.
	// Escape special regex chars in Open/Close.
	openEsc := regexp.QuoteMeta(syntax.Open)
	closeEsc := regexp.QuoteMeta(syntax.Close)
	// Inner part: any chars except the open delimiter, non-greedy before close.
	// We allow any characters (including spaces) inside the placeholder.
	patStr := openEsc + `[\s\S]*?` + closeEsc
	pat, err := regexp.Compile(patStr)
	if err != nil {
		// Fall back to default pattern on error.
		pat = placeholderRegex
	}

	// Find all matches in the concatenated rune string.
	// regexp operates on bytes, but our full string is valid UTF-8 so byte
	// indices from FindAllStringIndex map correctly via runeIndexFromByteIndex.
	// Pre-build the rune index mapping for correct charMap lookup.
	fullRunes := []rune(full)
	_ = fullRunes // used implicitly through charMap

	// We need rune-based indices. Build a byte→rune index map.
	byteToRune := make([]int, len(full)+1)
	ri := 0
	for bi := range full {
		byteToRune[bi] = ri
		ri++
	}
	byteToRune[len(full)] = ri

	locs := pat.FindAllStringIndex(full, -1)
	if len(locs) == 0 {
		return nil
	}

	var matches []PlaceholderMatch
	openLen := len([]rune(syntax.Open))
	closeLen := len([]rune(syntax.Close))

	for _, loc := range locs {
		startByte, endByte := loc[0], loc[1]
		startRune := byteToRune[startByte]
		endRune := byteToRune[endByte] // exclusive

		if startRune >= len(charMap) || endRune-1 >= len(charMap) {
			continue
		}

		startCP := charMap[startRune]
		endCP := charMap[endRune-1] // last rune of match (inclusive)

		// Extract the full text and inner key.
		fullText := string([]rune(full)[startRune:endRune])
		// Strip delimiters.
		innerRunes := []rune(fullText)
		if len(innerRunes) < openLen+closeLen {
			continue
		}
		inner := string(innerRunes[openLen : len(innerRunes)-closeLen])

		pType, key := classifyPlaceholder(inner)

		// Collect the distinct run elements that this match spans.
		firstRunIdx := startCP.runIndex
		lastRunIdx := endCP.runIndex

		spanRuns := runs[firstRunIdx : lastRunIdx+1]

		m := PlaceholderMatch{
			FullText:    fullText,
			Key:         key,
			Type:        pType,
			Runs:        spanRuns,
			StartRun:    firstRunIdx,
			EndRun:      lastRunIdx,
			StartOffset: startCP.offset,
			EndOffset:   endCP.offset + 1, // exclusive
		}
		matches = append(matches, m)
	}

	return matches
}
