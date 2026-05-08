package web

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var secretQueryKeys = map[string]struct{}{
	"api_key": {}, "apikey": {}, "api-key": {},
	"access_token": {}, "token": {}, "auth": {}, "signature": {}, "sig": {}, "secret": {}, "client_secret": {},
}

var jwtLike = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
var githubPATLike = regexp.MustCompile(`^gh[pousr]_[A-Za-z0-9_]{20,}$`)
var rawSecretParam = regexp.MustCompile(`(?i)([?&#;][^=&#;\s]*(?:api[_-]?key|access[_-]?token|client[_-]?secret|token|auth|signature|secret|sig|x-amz-[^=&#;\s]*)=)([^&#\s]*)`)
var rawSecretValue = regexp.MustCompile(`(?i)(gh[pousr]_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`)

func ContainsURLSecret(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawContainsURLSecret(rawURL)
	}
	q := u.Query()
	for k, vv := range q {
		lk := strings.ToLower(k)
		if _, ok := secretQueryKeys[lk]; ok {
			return true
		}
		if strings.HasPrefix(lk, "x-amz-") {
			return true
		}
		for _, v := range vv {
			if lk == "code" && looksSensitiveCodeValue(v) {
				return true
			}
			if looksSecretValueForKey(lk, v) {
				return true
			}
		}
	}
	if looksSecretValue(u.Fragment) {
		return true
	}
	if containsPathSecret(u.Path) {
		return true
	}
	return false
}

func RedactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return RedactRawURL(rawURL)
	}
	q := u.Query()
	for k, vv := range q {
		lk := strings.ToLower(k)
		redact := false
		if _, ok := secretQueryKeys[lk]; ok {
			redact = true
		}
		if strings.HasPrefix(lk, "x-amz-") {
			redact = true
		}
		if !redact {
			for _, v := range vv {
				if lk == "code" && looksSensitiveCodeValue(v) {
					redact = true
					break
				}
				if looksSecretValueForKey(lk, v) {
					redact = true
					break
				}
			}
		}
		if redact {
			q.Set(k, "[REDACTED]")
		}
	}
	u.RawQuery = canonicalQuery(q)
	if u.User != nil {
		u.User = url.User("[REDACTED]")
	}
	if looksSecretValue(u.Fragment) || strings.Contains(strings.ToLower(u.Fragment), "token") {
		u.Fragment = "[REDACTED]"
	}
	u.Path = redactSecretPath(u.Path)
	return u.String()
}

func rawContainsURLSecret(rawURL string) bool {
	return rawSecretParam.MatchString(rawURL) || rawSecretValue.MatchString(rawURL)
}

func RedactRawURL(rawURL string) string {
	redacted := rawSecretParam.ReplaceAllString(rawURL, "${1}[REDACTED]")
	redacted = rawSecretValue.ReplaceAllString(redacted, "[REDACTED]")
	return redacted
}

func CanonicalCacheURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	u.RawQuery = canonicalQuery(q)
	return u.String()
}

func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func looksSecretValue(v string) bool {
	lv := strings.ToLower(strings.TrimSpace(v))
	if lv == "" {
		return false
	}
	if jwtLike.MatchString(v) {
		return true
	}
	if strings.HasPrefix(lv, "sk-") {
		return true
	}
	if githubPATLike.MatchString(v) {
		return true
	}
	return false
}

func looksSecretValueForKey(key, v string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if looksSecretValue(v) {
		return true
	}
	// Keep ordinary search query values usable; only block high-confidence token shapes.
	switch key {
	case "q", "query", "search":
		return false
	default:
		return false
	}
}

func looksSensitiveCodeValue(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if decoded, err := url.QueryUnescape(v); err == nil {
		v = decoded
	}
	if looksSecretValue(v) {
		return true
	}
	if len(v) >= 32 && isAlphaNumeric(v) {
		return true
	}
	return len(v) >= 20 && strings.ContainsAny(v, "-_/.+=")
}

func isAlphaNumeric(v string) bool {
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return v != ""
}

func containsPathSecret(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		if unescaped, err := url.PathUnescape(part); err == nil {
			part = unescaped
		}
		if looksPathSecretValue(part) {
			return true
		}
	}
	return false
}

func redactSecretPath(path string) string {
	if path == "" {
		return path
	}
	parts := strings.Split(path, "/")
	changed := false
	for i, part := range parts {
		if part == "" {
			continue
		}
		check := part
		if unescaped, err := url.PathUnescape(part); err == nil {
			check = unescaped
		}
		if looksPathSecretValue(check) {
			parts[i] = "[REDACTED]"
			changed = true
		}
	}
	if !changed {
		return path
	}
	return strings.Join(parts, "/")
}

func looksPathSecretValue(v string) bool {
	lv := strings.ToLower(strings.TrimSpace(v))
	if lv == "" {
		return false
	}
	if jwtLike.MatchString(v) {
		return true
	}
	return strings.HasPrefix(lv, "ghp_") || strings.HasPrefix(lv, "sk-")
}
