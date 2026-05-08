package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type proxyResolver struct {
	cfg      ProxyConfig
	resolver dnsResolver
	envProxy func(*http.Request) (*url.URL, error)
}

func newProxyResolver(cfg ProxyConfig, resolver dnsResolver) *proxyResolver {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &proxyResolver{cfg: cfg, resolver: resolver, envProxy: http.ProxyFromEnvironment}
}

func (p *proxyResolver) mode() string {
	mode := strings.ToLower(strings.TrimSpace(p.cfg.Mode))
	if mode == "" {
		return "environment"
	}
	return mode
}

func (p *proxyResolver) decide(req *http.Request) (*url.URL, PolicyMetadata, error) {
	meta := PolicyMetadata{ProxyMode: p.mode()}
	switch p.mode() {
	case "direct":
		return nil, meta, nil
	case "explicit":
		if strings.TrimSpace(p.cfg.URL) == "" {
			return nil, meta, fmt.Errorf("web proxy mode explicit requires proxy.url")
		}
		u, err := url.Parse(p.cfg.URL)
		if err != nil {
			return nil, meta, fmt.Errorf("invalid web proxy URL: %w", err)
		}
		class, err := p.validateProxyEndpoint(u)
		if err != nil {
			return nil, meta, err
		}
		meta.ProxyUsed = true
		meta.ProxyEndpointClass = class
		return u, meta, nil
	case "environment":
		u, err := p.envProxy(req)
		if err != nil {
			return nil, meta, fmt.Errorf("failed to resolve proxy from environment: %w", err)
		}
		if u == nil {
			return nil, meta, nil
		}
		class, err := p.validateProxyEndpoint(u)
		if err != nil {
			return nil, meta, err
		}
		meta.ProxyUsed = true
		meta.ProxyEndpointClass = class
		return u, meta, nil
	default:
		return nil, meta, fmt.Errorf("unsupported web proxy mode %q", p.cfg.Mode)
	}
}

func (p *proxyResolver) validateProxyEndpoint(u *url.URL) (string, error) {
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported proxy scheme %q: only http/https are supported", scheme)
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return "", fmt.Errorf("invalid proxy endpoint: missing hostname")
	}
	if host == "localhost" {
		if p.cfg.AllowLocalProxy {
			return "loopback", nil
		}
		return "", fmt.Errorf("proxy endpoint %q is blocked by policy", u.Redacted())
	}
	if ip, ok := parseIPLikeHost(host); ok {
		class := ipClass(ip, URLPolicyConfig{BlockMetadataServices: true}, nil)
		switch class {
		case "public":
			return "public", nil
		case "loopback_or_linklocal":
			if p.cfg.AllowLocalProxy && ip.IsLoopback() {
				return "loopback", nil
			}
		}
		return "", fmt.Errorf("proxy endpoint %q is blocked by policy (%s)", u.Redacted(), class)
	}
	addrs, err := p.resolver.LookupIPAddr(context.Background(), host)
	if err != nil || len(addrs) == 0 {
		return "", fmt.Errorf("proxy endpoint %q DNS resolution failed", u.Redacted())
	}
	endpointClass := "public"
	for _, addr := range addrs {
		class := ipClass(addr.IP, URLPolicyConfig{BlockMetadataServices: true}, nil)
		if class == "public" {
			continue
		}
		if class == "loopback_or_linklocal" && p.cfg.AllowLocalProxy && addr.IP.IsLoopback() {
			endpointClass = "loopback"
			continue
		}
		return "", fmt.Errorf("proxy endpoint %q is blocked by policy (%s)", u.Redacted(), class)
	}
	return endpointClass, nil
}

func (p *proxyResolver) resolveAllowedProxyDialTargets(ctx context.Context, host, port string) ([]string, bool, error) {
	for _, proxyURL := range p.configuredProxyURLs() {
		if proxyURL == nil || proxyPort(proxyURL) != port {
			continue
		}
		if !p.sameProxyEndpointHost(ctx, host, proxyURL.Hostname()) {
			continue
		}
		if _, err := p.validateProxyEndpoint(proxyURL); err != nil {
			return nil, true, err
		}
		return []string{net.JoinHostPort(host, port)}, true, nil
	}
	return nil, false, nil
}

func (p *proxyResolver) configuredProxyURLs() []*url.URL {
	switch p.mode() {
	case "explicit":
		u, err := url.Parse(p.cfg.URL)
		if err != nil {
			return nil
		}
		return []*url.URL{u}
	case "environment":
		var urls []*url.URL
		seen := make(map[string]struct{})
		for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
			raw := strings.TrimSpace(os.Getenv(key))
			if raw == "" {
				continue
			}
			u, err := parseProxyURL(raw)
			if err != nil || u == nil {
				continue
			}
			canonical := u.String()
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			urls = append(urls, u)
		}
		return urls
	default:
		return nil
	}
}

func parseProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		u, err = url.Parse("http://" + raw)
		if err != nil {
			return nil, err
		}
	}
	return u, nil
}

func proxyPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	default:
		return "80"
	}
}

func (p *proxyResolver) sameProxyEndpointHost(ctx context.Context, dialHost, proxyHost string) bool {
	dialHost = strings.ToLower(strings.TrimSpace(stripHostBrackets(dialHost)))
	proxyHost = strings.ToLower(strings.TrimSpace(stripHostBrackets(proxyHost)))
	if dialHost == "" || proxyHost == "" {
		return false
	}
	if dialHost == proxyHost {
		return true
	}
	dialIP, dialIsIP := parseIPLikeHost(dialHost)
	proxyIP, proxyIsIP := parseIPLikeHost(proxyHost)
	if dialIsIP && proxyIsIP {
		return dialIP.Equal(proxyIP)
	}
	if dialIsIP && proxyHost == "localhost" {
		return dialIP.IsLoopback()
	}
	if proxyIsIP && dialHost == "localhost" {
		return proxyIP.IsLoopback()
	}
	if dialIsIP {
		return p.hostResolvesToIP(ctx, proxyHost, dialIP)
	}
	if proxyIsIP {
		return p.hostResolvesToIP(ctx, dialHost, proxyIP)
	}
	return false
}

func (p *proxyResolver) hostResolvesToIP(ctx context.Context, host string, ip net.IP) bool {
	addrs, err := p.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if addr.IP.Equal(ip) {
			return true
		}
	}
	return false
}
