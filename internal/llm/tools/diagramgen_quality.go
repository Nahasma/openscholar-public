package tools

import (
	"fmt"
	"regexp"
	"strings"
)

var strokeDashBoolRE = regexp.MustCompile(`(?m)(stroke-dash\s*:\s*)(true|false)\b`)
var edgeRefRE = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.-]*)\s*->\s*([A-Za-z_][A-Za-z0-9_.-]*)\b`)
var idDeclRE = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:`)
var containerOpenRE = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:[^{]*\{\s*$`)

func normalizeAndLintD2(code string, strict bool) (string, []string, []string) {
	warnings := make([]string, 0)
	errors := make([]string, 0)

	normalized := strokeDashBoolRE.ReplaceAllStringFunc(code, func(s string) string {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(s)), "true") {
			return strings.Replace(s, "true", "3", 1)
		}
		return strings.Replace(s, "false", "0", 1)
	})
	if normalized != code {
		warnings = append(warnings, "normalized style.stroke-dash boolean values to numeric values (true->3, false->0)")
	}

	containerByChild, rootIDs := scanD2Structure(normalized)
	rewritten := edgeRefRE.ReplaceAllStringFunc(normalized, func(match string) string {
		sub := edgeRefRE.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		src := qualifyIfUniqueNested(sub[1], containerByChild, rootIDs, strict, &warnings, &errors)
		dst := qualifyIfUniqueNested(sub[2], containerByChild, rootIDs, strict, &warnings, &errors)
		return src + " -> " + dst
	})

	return rewritten, dedupeStrings(warnings), dedupeStrings(errors)
}

func scanD2Structure(code string) (map[string][]string, map[string]struct{}) {
	containerByChild := map[string][]string{}
	rootIDs := map[string]struct{}{}
	stack := make([]string, 0)
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for strings.HasPrefix(trimmed, "}") {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "}"))
		}

		if m := containerOpenRE.FindStringSubmatch(trimmed); len(m) == 2 {
			if len(stack) == 0 {
				rootIDs[m[1]] = struct{}{}
				stack = append(stack, m[1])
			} else {
				path := stack[len(stack)-1] + "." + m[1]
				containerByChild[m[1]] = append(containerByChild[m[1]], path)
				stack = append(stack, path)
			}
			continue
		}

		if m := idDeclRE.FindStringSubmatch(trimmed); len(m) == 2 {
			id := m[1]
			if len(stack) == 0 {
				rootIDs[id] = struct{}{}
			} else {
				containerByChild[id] = append(containerByChild[id], stack[len(stack)-1]+"."+id)
			}
		}
	}
	return containerByChild, rootIDs
}

func qualifyIfUniqueNested(id string, containerByChild map[string][]string, rootIDs map[string]struct{}, strict bool, warnings, errors *[]string) string {
	if strings.Contains(id, ".") {
		return id
	}
	if _, ok := rootIDs[id]; ok {
		return id
	}
	parents := dedupeStrings(containerByChild[id])
	if len(parents) == 1 {
		*warnings = append(*warnings, fmt.Sprintf("autofixed bare nested reference %q to %q", id, parents[0]))
		return parents[0]
	}
	if len(parents) > 1 && strict {
		*errors = append(*errors, fmt.Sprintf("ambiguous bare nested reference %q appears at multiple paths (%s); use the full path", id, strings.Join(parents, ", ")))
	}
	return id
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
