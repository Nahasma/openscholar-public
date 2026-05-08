package command

import "strings"

// Invocation is a parsed slash-command input.
type Invocation struct {
	Raw                string
	Cursor             int
	IsSlash            bool
	Command            string
	ArgsRaw            string
	Argv               []string
	Subcommand         string
	TrailingSpace      bool
	CursorInCommand    bool
	CursorInSubcommand bool
	CommandPrefix      string
	SubcommandPrefix   string
}

// CompletionContext describes completion target at cursor.
type CompletionContext struct {
	Invocation Invocation
	Target     string // "command" | "subcommand" | ""
	Prefix     string
}

// CompletionItem is a UI-friendly completion candidate.
type CompletionItem struct {
	Kind         string // "command" | "subcommand"
	Value        string
	CommandName  string
	Subcommand   string
	Description  string
	ArgumentHint string
	WhenToUse    string
	Source       string
}

// ParseInvocation parses slash command text with basic quoted argv parsing.
func ParseInvocation(raw string, cursor int) Invocation {
	inv := Invocation{Raw: raw}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(raw) {
		cursor = len(raw)
	}
	inv.Cursor = cursor
	inv.TrailingSpace = strings.HasSuffix(raw, " ")

	if raw == "" || raw[0] != '/' {
		return inv
	}
	inv.IsSlash = true
	body := raw[1:]
	if body == "" {
		return inv
	}

	firstSpace := strings.IndexByte(body, ' ')
	if firstSpace < 0 {
		inv.Command = body
		inv.CommandPrefix = tokenPrefixAt(body, cursor-1)
		inv.CursorInCommand = true
		return inv
	}

	inv.Command = body[:firstSpace]
	rest := body[firstSpace+1:]
	inv.ArgsRaw = strings.TrimLeft(rest, " ")
	inv.Argv = splitQuoted(inv.ArgsRaw)
	if len(inv.Argv) > 0 {
		inv.Subcommand = inv.Argv[0]
	}

	spacePosInRaw := 1 + firstSpace
	inv.CursorInCommand = cursor <= spacePosInRaw
	if inv.CursorInCommand {
		inv.CommandPrefix = tokenPrefixAt(body[:firstSpace], cursor-1)
		return inv
	}

	after := raw[spacePosInRaw+1:]
	inv.SubcommandPrefix, inv.CursorInSubcommand = firstArgPrefixAt(after, cursor-(spacePosInRaw+1))
	return inv
}

func firstArgPrefixAt(raw string, cursor int) (string, bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(raw) {
		cursor = len(raw)
	}
	left := raw[:cursor]
	left = strings.TrimLeft(left, " ")
	if left == "" {
		return "", true
	}
	if idx := strings.IndexByte(left, ' '); idx >= 0 {
		return "", false
	}
	return left, true
}

func tokenPrefixAt(raw string, cursor int) string {
	if cursor < 0 {
		return ""
	}
	if cursor > len(raw) {
		cursor = len(raw)
	}
	left := raw[:cursor]
	if strings.Contains(left, " ") {
		parts := strings.Split(left, " ")
		return parts[len(parts)-1]
	}
	return left
}

func splitQuoted(raw string) []string {
	var out []string
	var b strings.Builder
	var quote byte
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			if ch == '\\' && i+1 < len(raw) && raw[i+1] == quote {
				i++
				b.WriteByte(raw[i])
				continue
			}
			b.WriteByte(ch)
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case ' ', '\t', '\n':
			flush()
		default:
			b.WriteByte(ch)
		}
	}
	flush()
	return out
}
