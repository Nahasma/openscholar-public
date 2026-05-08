package web

import "strings"

type WebsitePolicy struct {
	cfg WebsitePolicyConfig
}

func NewWebsitePolicy(cfg WebsitePolicyConfig) *WebsitePolicy {
	return &WebsitePolicy{cfg: cfg}
}

func (p *WebsitePolicy) Check(host string) error {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil
	}
	matched := false
	action := ""
	for _, r := range p.cfg.Rules {
		if matchWebsitePattern(host, strings.ToLower(strings.TrimSpace(r.Pattern))) {
			matched = true
			action = strings.ToLower(strings.TrimSpace(r.Action))
			break
		}
	}

	mode := strings.ToLower(strings.TrimSpace(p.cfg.Mode))
	def := strings.ToLower(strings.TrimSpace(p.cfg.DefaultAction))
	if def == "" {
		if mode == "allowlist" {
			def = "block"
		} else {
			def = "allow"
		}
	}
	if matched {
		if action == "block" {
			return ErrBlockedByWebsitePolicy
		}
		return nil
	}
	if def == "block" {
		return ErrBlockedByWebsitePolicy
	}
	return nil
}

func matchWebsitePattern(host, pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		base := strings.TrimPrefix(pattern, "*.")
		return host == base || strings.HasSuffix(host, "."+base)
	}
	if strings.HasPrefix(pattern, ".") {
		base := strings.TrimPrefix(pattern, ".")
		return host == base || strings.HasSuffix(host, "."+base)
	}
	return host == pattern || strings.HasSuffix(host, "."+pattern)
}
