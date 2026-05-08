package web

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

// Constants aligned with Claude Code's WebFetchTool/utils.ts.
const (
	// MaxURLLength is the maximum permitted URL length.
	// PSR originally requested 250, but CC raised it to 2000 to support
	// JWT-signed URLs (cloud service signed URLs) for legitimate use cases.
	MaxURLLength = 2000

	// MaxRedirects caps same-host redirect hops to prevent redirect loops.
	// 10 matches common client defaults (axios=5, Chrome=20).
	MaxRedirects = 10

	// FetchTimeout is the timeout for the main HTTP fetch request.
	// Prevents hanging indefinitely on slow/unresponsive servers.
	FetchTimeout = 60 * time.Second

	// MaxContentLength limits response body size to prevent resource exhaustion.
	// Per PSR: "Implement resource consumption controls".
	MaxContentLength = 10 * 1024 * 1024 // 10 MB
)

// privateIPNets enumerates RFC-1918, loopback, link-local, and multicast
// ranges that SSRF attacks commonly target.
var privateIPNets []*net.IPNet

func init() {
	cidrs := []string{
		// IPv4 loopback
		"127.0.0.0/8",
		// IPv4 private (RFC 1918)
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// IPv4 link-local
		"169.254.0.0/16",
		// IPv4 multicast
		"224.0.0.0/4",
		// IPv4 "this" network
		"0.0.0.0/8",
		// IPv6 loopback
		"::1/128",
		// IPv6 link-local
		"fe80::/10",
		// IPv6 unique local (RFC 4193)
		"fc00::/7",
		// IPv6 multicast
		"ff00::/8",
	}
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPNets = append(privateIPNets, ipNet)
		}
	}
}

// isPrivateIP returns true when ip falls within a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	for _, block := range privateIPNets {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateWebURL validates rawURL for safety before fetching.
//
// Checks performed (aligned with CC's validateURL + SSRF notes):
//   - Length ≤ MaxURLLength
//   - Parseable by net/url
//   - Scheme is http or https
//   - No userinfo (username/password)
//   - Hostname contains at least one "." (rules out bare hostnames / .local)
//   - Hostname is not "localhost"
//   - If hostname is a literal IP, it must not be a private/reserved address
//   - If hostname is a DNS name, attempt resolution and reject if all resolved
//     IPs are private/reserved
func ValidateWebURL(rawURL string) error {
	return NewURLPolicy(DefaultConfig().URLPolicy).Check(context.Background(), rawURL, PurposeFetch)
}

// IsPermittedRedirect reports whether following a redirect from originalURL to
// redirectURL is safe.
//
// Rules (strictly aligned with CC's isPermittedRedirect):
//   - Same protocol (scheme)
//   - Same port
//   - No userinfo in the redirect URL
//   - Hostnames match after stripping a leading "www." from either side
//     (allows example.com → www.example.com and vice-versa)
func IsPermittedRedirect(originalURL, redirectURL string) bool {
	parsedOriginal, err := url.Parse(originalURL)
	if err != nil {
		return false
	}

	parsedRedirect, err := url.Parse(redirectURL)
	if err != nil {
		return false
	}

	// Protocol must match.
	if !strings.EqualFold(parsedRedirect.Scheme, parsedOriginal.Scheme) {
		return false
	}

	// Port must match (empty string == default port for the scheme).
	if parsedRedirect.Port() != parsedOriginal.Port() {
		return false
	}

	// No credentials in the redirect target.
	if parsedRedirect.User != nil {
		return false
	}

	// Hostname comparison: strip "www." prefix before comparing.
	stripWWW := func(h string) string {
		return strings.TrimPrefix(strings.ToLower(h), "www.")
	}

	return stripWWW(parsedOriginal.Hostname()) == stripWWW(parsedRedirect.Hostname())
}
