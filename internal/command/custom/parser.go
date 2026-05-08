package custom

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the YAML frontmatter of a custom command file.
type Frontmatter struct {
	Name                      string   `yaml:"name"`
	Description               string   `yaml:"description"`
	AllowedTools              []string `yaml:"allowed_tools"`            // restrict which tools the command may use
	Model                     string   `yaml:"model"`                    // override model for this command (alias or ID)
	WhenToUse                 string   `yaml:"when_to_use"`              // hint for automatic command selection
	ArgumentHint              string   `yaml:"argument_hint"`            // displayed in help (e.g. "[paper-id]")
	Arguments                 []string `yaml:"arguments"`                // named argument list for $name substitution
	UserInvocable             *bool    `yaml:"user_invocable"`           // nil = default true
	DisableModelInvocation    bool     `yaml:"disable_model_invocation"` // runtime guard for slash-command model execution
	DisableModelInvocationAlt bool     `yaml:"disable-model-invocation"` // kebab-case compatibility
}

// ParseFrontmatter splits a markdown file into frontmatter and body.
// Returns an error only for YAML decode failures; missing/empty frontmatter is not an error.
func ParseFrontmatter(content string) (Frontmatter, string, error) {
	var fm Frontmatter
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return fm, content, nil
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fm, content, nil
	}

	fmRaw := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return fm, body, fmt.Errorf("frontmatter parse error: %w", err)
	}
	return fm, body, nil
}

// ExpandBody replaces template variables in the command body.
//
// Replacement order (highest priority first):
//  1. Named args: $<argname> → value from args split by position (per fm.Arguments)
//  2. Indexed args: $ARGUMENTS[N] → Nth whitespace-delimited token from args
//  3. $ARGUMENTS → the full args string
//  4. @path → contents of file at path
//  5. !`cmd` → stdout of running cmd in shell
func ExpandBody(body string, args string) string {
	return expandBodyWithFrontmatter(body, args, Frontmatter{})
}

// ExpandBodyFM is like ExpandBody but uses frontmatter metadata (Arguments list, etc.).
func ExpandBodyFM(body string, args string, fm Frontmatter) string {
	return expandBodyWithFrontmatter(body, args, fm)
}

func expandBodyWithFrontmatter(body string, args string, fm Frontmatter) string {
	// Split args into positional tokens for indexed / named substitution
	tokens := splitArgs(args)

	// 1. Named argument substitution: $<name> → token by position in fm.Arguments
	for i, argName := range fm.Arguments {
		placeholder := "$" + argName
		value := ""
		if i < len(tokens) {
			value = tokens[i]
		}
		body = strings.ReplaceAll(body, placeholder, value)
	}

	// 2. Indexed substitution: $ARGUMENTS[N]
	indexedRe := regexp.MustCompile(`\$ARGUMENTS\[(\d+)\]`)
	body = indexedRe.ReplaceAllStringFunc(body, func(match string) string {
		sub := indexedRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		idx := 0
		fmt.Sscanf(sub[1], "%d", &idx)
		if idx < len(tokens) {
			return tokens[idx]
		}
		return ""
	})

	// 3. Replace $ARGUMENTS with full arg string
	body = strings.ReplaceAll(body, "$ARGUMENTS", args)

	// 4. Replace !`cmd` patterns (shell execution)
	shellRe := regexp.MustCompile("!`([^`]+)`")
	body = shellRe.ReplaceAllStringFunc(body, func(match string) string {
		cmd := shellRe.FindStringSubmatch(match)[1]
		out, err := exec.Command("sh", "-c", cmd).Output()
		if err != nil {
			return "(error: " + err.Error() + ")"
		}
		return strings.TrimSpace(string(out))
	})

	// 5. Replace @path patterns (file inclusion)
	pathRe := regexp.MustCompile(`@(\S+)`)
	body = pathRe.ReplaceAllStringFunc(body, func(match string) string {
		path := match[1:] // strip @
		data, err := os.ReadFile(path)
		if err != nil {
			return "(error reading " + path + ": " + err.Error() + ")"
		}
		return string(data)
	})

	return body
}

// splitArgs splits an argument string into whitespace-delimited tokens,
// respecting double-quoted groups.
func splitArgs(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	// Simple split on whitespace (quoted strings are kept together)
	var tokens []string
	inQuote := false
	var cur strings.Builder
	for _, r := range args {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
