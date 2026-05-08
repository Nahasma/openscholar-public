package permission

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Decision represents a permission evaluation result.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

// Rule defines a single permission rule matching a tool and pattern.
type Rule struct {
	Tool     string   // "Edit", "Write", "Bash", "View", "Glob", "Grep", "*"
	Pattern  string   // glob for file tools, prefix+wildcard for Bash
	Decision Decision // "allow", "deny", "ask"
}

// DefaultDenyRules returns built-in deny rules protecting sensitive files and blocking dangerous commands.
func DefaultDenyRules() []Rule {
	return []Rule{
		// Sensitive files — all tools denied
		{Tool: "*", Pattern: ".env", Decision: Deny},
		{Tool: "*", Pattern: ".env.*", Decision: Deny},
		{Tool: "*", Pattern: "**/secrets/**", Decision: Deny},
		{Tool: "*", Pattern: "**/*credential*", Decision: Deny},
		{Tool: "*", Pattern: "**/*secret*", Decision: Deny},
		{Tool: "*", Pattern: "**/*.pem", Decision: Deny},
		{Tool: "*", Pattern: "**/*.key", Decision: Deny},
		{Tool: "*", Pattern: "**/*.p12", Decision: Deny},
		{Tool: "*", Pattern: "**/*id_rsa*", Decision: Deny},

		// Dangerous Bash commands
		{Tool: "Bash", Pattern: "rm -rf *", Decision: Deny},
		{Tool: "Bash", Pattern: "git push --force *", Decision: Deny},
		{Tool: "Bash", Pattern: "git push -f *", Decision: Deny},
		{Tool: "Bash", Pattern: "git reset --hard *", Decision: Deny},
		{Tool: "Bash", Pattern: "git clean -f *", Decision: Deny},
		{Tool: "Bash", Pattern: "chmod 777 *", Decision: Deny},
		{Tool: "Bash", Pattern: "> *", Decision: Deny},
	}
}

// DefaultAllowRules returns built-in allow rules for common safe operations.
func DefaultAllowRules() []Rule {
	return []Rule{
		// Read operations — auto-approve within workspace
		{Tool: "View", Pattern: "**", Decision: Allow},
		{Tool: "Glob", Pattern: "**", Decision: Allow},
		{Tool: "Grep", Pattern: "**", Decision: Allow},

		// Write operations — auto-approve within workspace (deny rules protect sensitive files)
		{Tool: "Edit", Pattern: "**", Decision: Allow},
		{Tool: "Write", Pattern: "**", Decision: Allow},

		// Safe Bash — read-only / info commands
		{Tool: "Bash", Pattern: "ls *", Decision: Allow},
		{Tool: "Bash", Pattern: "cat *", Decision: Allow},
		{Tool: "Bash", Pattern: "head *", Decision: Allow},
		{Tool: "Bash", Pattern: "tail *", Decision: Allow},
		{Tool: "Bash", Pattern: "wc *", Decision: Allow},
		{Tool: "Bash", Pattern: "find *", Decision: Allow},
		{Tool: "Bash", Pattern: "grep *", Decision: Allow},
		{Tool: "Bash", Pattern: "rg *", Decision: Allow},
		{Tool: "Bash", Pattern: "pwd", Decision: Allow},
		{Tool: "Bash", Pattern: "which *", Decision: Allow},
		{Tool: "Bash", Pattern: "echo *", Decision: Allow},
		{Tool: "Bash", Pattern: "date", Decision: Allow},
		{Tool: "Bash", Pattern: "env", Decision: Allow},
		{Tool: "Bash", Pattern: "sort *", Decision: Allow},
		{Tool: "Bash", Pattern: "diff *", Decision: Allow},
		{Tool: "Bash", Pattern: "tree *", Decision: Allow},
		{Tool: "Bash", Pattern: "mkdir *", Decision: Allow},
		{Tool: "Bash", Pattern: "touch *", Decision: Allow},
		{Tool: "Bash", Pattern: "cp *", Decision: Allow},
		{Tool: "Bash", Pattern: "mv *", Decision: Allow},

		// Git — read-only + common write operations
		{Tool: "Bash", Pattern: "git status *", Decision: Allow},
		{Tool: "Bash", Pattern: "git status", Decision: Allow},
		{Tool: "Bash", Pattern: "git log *", Decision: Allow},
		{Tool: "Bash", Pattern: "git log", Decision: Allow},
		{Tool: "Bash", Pattern: "git diff *", Decision: Allow},
		{Tool: "Bash", Pattern: "git diff", Decision: Allow},
		{Tool: "Bash", Pattern: "git show *", Decision: Allow},
		{Tool: "Bash", Pattern: "git branch *", Decision: Allow},
		{Tool: "Bash", Pattern: "git branch", Decision: Allow},
		{Tool: "Bash", Pattern: "git add *", Decision: Allow},
		{Tool: "Bash", Pattern: "git commit *", Decision: Allow},
		{Tool: "Bash", Pattern: "git checkout *", Decision: Allow},
		{Tool: "Bash", Pattern: "git merge *", Decision: Allow},
		{Tool: "Bash", Pattern: "git rebase *", Decision: Allow},
		{Tool: "Bash", Pattern: "git stash *", Decision: Allow},
		{Tool: "Bash", Pattern: "git stash", Decision: Allow},
		{Tool: "Bash", Pattern: "git fetch *", Decision: Allow},
		{Tool: "Bash", Pattern: "git fetch", Decision: Allow},
		{Tool: "Bash", Pattern: "git pull *", Decision: Allow},
		{Tool: "Bash", Pattern: "git pull", Decision: Allow},
		{Tool: "Bash", Pattern: "git remote *", Decision: Allow},
		{Tool: "Bash", Pattern: "git blame *", Decision: Allow},
		{Tool: "Bash", Pattern: "git rev-parse *", Decision: Allow},
		{Tool: "Bash", Pattern: "git tag *", Decision: Allow},

		// Go dev toolchain
		{Tool: "Bash", Pattern: "go build *", Decision: Allow},
		{Tool: "Bash", Pattern: "go test *", Decision: Allow},
		{Tool: "Bash", Pattern: "go run *", Decision: Allow},
		{Tool: "Bash", Pattern: "go mod *", Decision: Allow},
		{Tool: "Bash", Pattern: "go vet *", Decision: Allow},
		{Tool: "Bash", Pattern: "go fmt *", Decision: Allow},
		{Tool: "Bash", Pattern: "go generate *", Decision: Allow},
		{Tool: "Bash", Pattern: "go install *", Decision: Allow},
		{Tool: "Bash", Pattern: "go env *", Decision: Allow},
		{Tool: "Bash", Pattern: "go env", Decision: Allow},
		{Tool: "Bash", Pattern: "go version", Decision: Allow},
		{Tool: "Bash", Pattern: "go list *", Decision: Allow},

		// Build / script commands
		{Tool: "Bash", Pattern: "make *", Decision: Allow},
		{Tool: "Bash", Pattern: "make", Decision: Allow},
		{Tool: "Bash", Pattern: "npm *", Decision: Allow},
		{Tool: "Bash", Pattern: "yarn *", Decision: Allow},
		{Tool: "Bash", Pattern: "pnpm *", Decision: Allow},
		{Tool: "Bash", Pattern: "python *", Decision: Allow},
		{Tool: "Bash", Pattern: "python3 *", Decision: Allow},
		{Tool: "Bash", Pattern: "pip *", Decision: Allow},
		{Tool: "Bash", Pattern: "pip3 *", Decision: Allow},
		{Tool: "Bash", Pattern: "cargo *", Decision: Allow},
		{Tool: "Bash", Pattern: "rustc *", Decision: Allow},
		{Tool: "Bash", Pattern: "docker *", Decision: Allow},
		{Tool: "Bash", Pattern: "docker-compose *", Decision: Allow},
		{Tool: "Bash", Pattern: "sqlc *", Decision: Allow},
		{Tool: "Bash", Pattern: "tectonic *", Decision: Allow},
		{Tool: "Bash", Pattern: "latexmk *", Decision: Allow},
		{Tool: "Bash", Pattern: "pdflatex *", Decision: Allow},
		{Tool: "Bash", Pattern: "xelatex *", Decision: Allow},
		{Tool: "Bash", Pattern: "bibtex *", Decision: Allow},
		{Tool: "Bash", Pattern: "biber *", Decision: Allow},
	}
}

// MatchFileRule checks if a file path matches a file-based rule pattern.
// The filePath should be relative to the workspace directory.
func MatchFileRule(rule Rule, filePath string) bool {
	// Normalize case on case-insensitive filesystems (darwin, windows)
	pattern := rule.Pattern
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		pattern = strings.ToLower(pattern)
		filePath = strings.ToLower(filePath)
	}

	// Normalize path separators
	filePath = filepath.ToSlash(filePath)
	pattern = filepath.ToSlash(pattern)

	// Try matching with doublestar (supports **)
	matched, err := doublestar.Match(pattern, filePath)
	if err != nil {
		return false
	}
	if matched {
		return true
	}

	// Also try matching against just the basename (for patterns like ".env")
	base := filepath.Base(filePath)
	matched, err = doublestar.Match(pattern, base)
	if err != nil {
		return false
	}
	return matched
}

// MatchBashRule checks if a command matches a Bash rule pattern.
// Supports:
//   - "pattern *" (trailing space+star): prefix match with word boundary
//   - "pattern*" (trailing star, no space): pure prefix match
//   - no star: exact match
//
// For compound commands (&&, ||, ;, |), each subcommand is evaluated independently.
// If ANY subcommand matches a deny rule, the whole command matches.
func MatchBashRule(rule Rule, command string) bool {
	cmd := strings.TrimSpace(command)
	pattern := rule.Pattern

	// For deny rules, split compound commands and check each subcommand
	if rule.Decision == Deny {
		subcommands := splitCompoundCommand(cmd)
		if len(subcommands) > 1 {
			for _, sub := range subcommands {
				if matchSingleBashPattern(pattern, strings.TrimSpace(sub)) {
					return true
				}
			}
			return false
		}
	}

	return matchSingleBashPattern(pattern, cmd)
}

// matchSingleBashPattern matches a single command against a bash pattern.
func matchSingleBashPattern(pattern, cmd string) bool {
	if prefix, ok := strings.CutSuffix(pattern, " *"); ok {
		// "prefix *" — prefix match with word boundary
		// "ls *" matches "ls -la" and "ls" but not "lsof"
		return cmd == prefix || strings.HasPrefix(cmd, prefix+" ")
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		// "prefix*" — pure prefix match (no word boundary)
		return strings.HasPrefix(cmd, prefix)
	}
	// Exact match
	return cmd == pattern
}

// splitCompoundCommand splits a command string by shell operators (&&, ||, ;, |).
// Returns the original command in a single-element slice if no operators found.
func splitCompoundCommand(cmd string) []string {
	var parts []string
	var current strings.Builder
	i := 0
	inSingleQuote := false
	inDoubleQuote := false

	for i < len(cmd) {
		ch := cmd[i]

		// Track quoting state
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
			i++
			continue
		}

		// Skip splitting inside quotes
		if inSingleQuote || inDoubleQuote {
			current.WriteByte(ch)
			i++
			continue
		}

		// Check for operators: &&, ||, ;, |
		if ch == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
			parts = append(parts, current.String())
			current.Reset()
			i += 2
			continue
		}
		if ch == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
			parts = append(parts, current.String())
			current.Reset()
			i += 2
			continue
		}
		if ch == ';' {
			parts = append(parts, current.String())
			current.Reset()
			i++
			continue
		}
		if ch == '|' {
			parts = append(parts, current.String())
			current.Reset()
			i++
			continue
		}

		current.WriteByte(ch)
		i++
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// Evaluate evaluates a target against a set of rules in priority order: deny → ask → allow.
// Returns the Decision of the first matching rule, or Ask if no rule matches.
// For file tools, target is the workspace-relative file path.
// For Bash, target is the command string.
func Evaluate(rules []Rule, toolName string, target string) Decision {
	// Phase 1: check deny rules
	for _, rule := range rules {
		if rule.Decision != Deny {
			continue
		}
		if matchRule(rule, toolName, target) {
			return Deny
		}
	}

	// Phase 2: check ask rules
	for _, rule := range rules {
		if rule.Decision != Ask {
			continue
		}
		if matchRule(rule, toolName, target) {
			return Ask
		}
	}

	// Phase 3: check allow rules
	for _, rule := range rules {
		if rule.Decision != Allow {
			continue
		}
		if matchRule(rule, toolName, target) {
			return Allow
		}
	}

	// No rule matched
	return Ask
}

// matchRule checks if a rule matches a given tool name and target.
func matchRule(rule Rule, toolName string, target string) bool {
	// Check tool match: "*" matches all tools, otherwise exact match
	if rule.Tool != "*" && rule.Tool != toolName {
		return false
	}

	// Match based on tool type
	if toolName == "Bash" {
		return MatchBashRule(rule, target)
	}
	// File tools: Edit, Write, View, Glob, Grep
	return MatchFileRule(rule, target)
}

// MakeRelativePath converts an absolute path to workspace-relative for rule matching.
// If the path is already relative or cannot be made relative, returns it as-is.
func MakeRelativePath(absPath, workspaceDir string) string {
	if workspaceDir == "" || !filepath.IsAbs(absPath) {
		return absPath
	}
	rel, err := filepath.Rel(workspaceDir, absPath)
	if err != nil {
		return absPath
	}
	// If the result starts with "..", the path is outside workspace
	if strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}
