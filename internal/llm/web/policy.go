package web

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type URLPurpose string

const (
	PurposeFetch    URLPurpose = "fetch"
	PurposeSearch   URLPurpose = "search"
	PurposeRedirect URLPurpose = "redirect"
)

type dnsResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type URLPolicy struct {
	cfg      URLPolicyConfig
	resolver dnsResolver
}

type PolicyMetadata struct {
	ProxyMode          string `json:"proxy_mode,omitempty"`
	ProxyUsed          bool   `json:"proxy_used,omitempty"`
	ProxyEndpointClass string `json:"proxy_endpoint_class,omitempty"`
	ResolvedIPClass    string `json:"resolved_ip_class,omitempty"`
	PolicyDecision     string `json:"policy_decision,omitempty"`
	FakeIPAllowed      bool   `json:"fake_ip_allowed,omitempty"`
}

type PolicyError struct {
	Message  string
	Metadata PolicyMetadata
}

func (e *PolicyError) Error() string { return e.Message }

type URLCheckOptions struct {
	PolicyMetadata
	FakeIPCIDRs []string
	Metadata    *PolicyMetadata
}

func NewURLPolicy(cfg URLPolicyConfig) *URLPolicy {
	return &URLPolicy{cfg: cfg, resolver: net.DefaultResolver}
}

func (p *URLPolicy) withResolver(resolver dnsResolver) *URLPolicy {
	if resolver != nil {
		p.resolver = resolver
	}
	return p
}

func (p *URLPolicy) Check(ctx context.Context, rawURL string, _ URLPurpose) error {
	return p.CheckWithOptions(ctx, rawURL, URLCheckOptions{})
}

func (p *URLPolicy) CheckWithOptions(ctx context.Context, rawURL string, opts URLCheckOptions) error {
	if len(rawURL) > MaxURLLength {
		return fmt.Errorf("URL exceeds maximum length of %d characters", MaxURLLength)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL is not parseable: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL scheme %q is not permitted; only http and https are allowed", u.Scheme)
	}

	if p.cfg.BlockUserinfo && u.User != nil {
		return fmt.Errorf("URL must not contain username or password")
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return fmt.Errorf("URL hostname is empty")
	}

	if isMetadataHost(hostname) {
		return &PolicyError{
			Message:  fmt.Sprintf("URL hostname %q is not permitted (metadata service)", hostname),
			Metadata: blockedMeta(opts.PolicyMetadata, "metadata", "blocked_target_metadata", false),
		}
	}

	if hostname == "localhost" {
		return &PolicyError{
			Message:  fmt.Sprintf("URL hostname %q is not permitted (loopback alias)", hostname),
			Metadata: blockedMeta(opts.PolicyMetadata, "loopback", "blocked_target_loopback", false),
		}
	}

	if ip, ok := parseIPLikeHost(hostname); ok {
		class := ipClass(ip, p.cfg, opts.FakeIPCIDRs)
		if class != "public" {
			return &PolicyError{
				Message:  fmt.Sprintf("URL targets a private or reserved IP address (%s)", ip.String()),
				Metadata: blockedMeta(opts.PolicyMetadata, class, "blocked_target_ip_literal", false),
			}
		}
		setPolicyMetadata(opts.Metadata, allowedMeta(opts.PolicyMetadata, class, "allowed_target_ip_literal", false))
		return nil
	}

	if !strings.Contains(hostname, ".") {
		return fmt.Errorf("URL hostname %q is not publicly resolvable (no dot separator)", hostname)
	}

	addrs, err := p.resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		if strings.EqualFold(p.cfg.DNSFailMode, "open_dev_only") {
			return nil
		}
		return fmt.Errorf("URL hostname %q DNS resolution failed", hostname)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("URL hostname %q DNS returned no addresses", hostname)
	}
	for _, addr := range addrs {
		class := ipClass(addr.IP, p.cfg, opts.FakeIPCIDRs)
		if class != "public" {
			allowFake := class == "fake_ip" && opts.ProxyUsed
			if allowFake {
				setPolicyMetadata(opts.Metadata, allowedMeta(opts.PolicyMetadata, class, "allowed_target_dns_fake_ip_via_proxy", true))
				continue
			}
			return &PolicyError{
				Message:  fmt.Sprintf("URL hostname %q resolves to private or reserved IP (%s)", hostname, addr.IP.String()),
				Metadata: blockedMeta(opts.PolicyMetadata, class, "blocked_target_dns_resolution", false),
			}
		}
		setPolicyMetadata(opts.Metadata, allowedMeta(opts.PolicyMetadata, class, "allowed_target_dns_resolution", false))
	}
	return nil
}

func allowedMeta(base PolicyMetadata, ipClass, decision string, fakeAllowed bool) PolicyMetadata {
	base.ResolvedIPClass = ipClass
	base.PolicyDecision = decision
	base.FakeIPAllowed = fakeAllowed
	return base
}

func blockedMeta(base PolicyMetadata, ipClass, decision string, fakeAllowed bool) PolicyMetadata {
	base.ResolvedIPClass = ipClass
	base.PolicyDecision = decision
	base.FakeIPAllowed = fakeAllowed
	return base
}

func setPolicyMetadata(dst *PolicyMetadata, meta PolicyMetadata) {
	if dst != nil {
		*dst = meta
	}
}

func PolicyMetadataFromError(err error) (PolicyMetadata, bool) {
	var policyErr *PolicyError
	if errors.As(err, &policyErr) {
		return policyErr.Metadata, true
	}
	return PolicyMetadata{}, false
}

func parseIPLikeHost(host string) (net.IP, bool) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, true
	}
	if ip := parseObfuscatedIPv4(host); ip != nil {
		return ip, true
	}
	return nil, false
}

func parseObfuscatedIPv4(host string) net.IP {
	if strings.Contains(host, ":") {
		return nil
	}
	if strings.Contains(host, ".") {
		parts := strings.Split(host, ".")
		if len(parts) != 4 {
			return nil
		}
		buf := make([]byte, 4)
		for i, p := range parts {
			if p == "" {
				return nil
			}
			v, err := strconv.ParseUint(p, 0, 8)
			if err != nil {
				return nil
			}
			buf[i] = byte(v)
		}
		return net.IPv4(buf[0], buf[1], buf[2], buf[3])
	}
	v, err := strconv.ParseUint(host, 0, 32)
	if err != nil {
		return nil
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(v))
	return net.IPv4(buf[0], buf[1], buf[2], buf[3])
}

func isMetadataHost(host string) bool {
	h := strings.TrimSuffix(host, ".")
	switch h {
	case "metadata", "metadata.google.internal", "metadata.goog", "169.254.169.254", "100.100.100.200", "instance-data", "instance-data.ec2.internal":
		return true
	default:
		return false
	}
}

func isBlockedIP(ip net.IP, cfg URLPolicyConfig) bool {
	return ipClass(ip, cfg, nil) != "public"
}

func ipClass(ip net.IP, cfg URLPolicyConfig, fakeIPCIDRs []string) string {
	if ip == nil {
		return "unknown"
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return "loopback_or_linklocal"
	}
	if ip.IsPrivate() {
		return "private"
	}
	if isInCIDR(ip, "198.18.0.0/15") || isInAnyCIDR(ip, fakeIPCIDRs) {
		return "fake_ip"
	}
	if isInCIDR(ip, "0.0.0.0/8") ||
		isInCIDR(ip, "100.64.0.0/10") ||
		isInCIDR(ip, "192.0.0.0/24") ||
		isInCIDR(ip, "192.0.2.0/24") ||
		isInCIDR(ip, "198.51.100.0/24") ||
		isInCIDR(ip, "203.0.113.0/24") ||
		isInCIDR(ip, "192.88.99.0/24") ||
		isInCIDR(ip, "2001::/32") ||
		isInCIDR(ip, "2001:db8::/32") ||
		isInCIDR(ip, "2002::/16") ||
		isInCIDR(ip, "fec0::/10") {
		return "reserved"
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] >= 240 {
			return "reserved"
		}
	}
	if cfg.BlockMetadataServices && (ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("100.100.100.200"))) {
		return "metadata"
	}
	return "public"
}

func isInCIDR(ip net.IP, cidr string) bool {
	_, block, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return block.Contains(ip)
}

func isInAnyCIDR(ip net.IP, cidrs []string) bool {
	for _, cidr := range cidrs {
		if isInCIDR(ip, cidr) {
			return true
		}
	}
	return false
}
