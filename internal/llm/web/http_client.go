package web

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

type policyRoundTripper struct {
	base          http.RoundTripper
	urlPolicy     *URLPolicy
	websitePolicy *WebsitePolicy
	blockSecrets  bool
	purpose       URLPurpose
	proxy         *proxyResolver
}

func newPolicyHTTPClient(timeout time.Duration, cfg Config, purpose URLPurpose) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newPolicyRoundTripper(NewURLPolicy(cfg.URLPolicy), NewWebsitePolicy(cfg.WebsitePolicy), cfg, purpose),
	}
}

func newPolicyRoundTripper(urlPolicy *URLPolicy, websitePolicy *WebsitePolicy, cfg Config, purpose URLPurpose) http.RoundTripper {
	if urlPolicy == nil {
		urlPolicy = NewURLPolicy(DefaultConfig().URLPolicy)
	}
	if websitePolicy == nil {
		websitePolicy = NewWebsitePolicy(DefaultConfig().WebsitePolicy)
	}
	if cfg.Proxy.Mode == "" {
		cfg.Proxy.Mode = DefaultConfig().Proxy.Mode
	}
	if len(cfg.Proxy.FakeIPCIDRs) == 0 {
		cfg.Proxy.FakeIPCIDRs = append([]string(nil), DefaultConfig().Proxy.FakeIPCIDRs...)
	}
	proxy := newProxyResolver(cfg.Proxy, urlPolicy.resolver)
	dialer := &policyDialer{urlPolicy: urlPolicy, proxy: proxy}
	return &policyRoundTripper{
		base: &http.Transport{
			DialContext:         dialer.DialContext,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			Proxy: func(req *http.Request) (*url.URL, error) {
				u, _, err := proxy.decide(req)
				return u, err
			},
		},
		urlPolicy:     urlPolicy,
		websitePolicy: websitePolicy,
		blockSecrets:  cfg.URLPolicy.BlockURLSecrets,
		purpose:       purpose,
		proxy:         proxy,
	}
}

func (t *policyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rawURL := req.URL.String()
	if t.blockSecrets && ContainsURLSecret(rawURL) {
		return nil, fmt.Errorf("URL contains sensitive parameters")
	}
	_, meta, err := t.proxy.decide(req)
	if err != nil {
		return nil, &PolicyError{
			Message:  err.Error(),
			Metadata: blockedMeta(meta, "", "blocked_proxy_endpoint", false),
		}
	}
	if err := t.urlPolicy.CheckWithOptions(req.Context(), rawURL, URLCheckOptions{
		PolicyMetadata: meta,
		FakeIPCIDRs:    t.proxy.cfg.FakeIPCIDRs,
	}); err != nil {
		return nil, err
	}
	if err := t.websitePolicy.Check(req.URL.Hostname()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

type policyDialer struct {
	urlPolicy *URLPolicy
	proxy     *proxyResolver
	dialer    net.Dialer
}

func (d *policyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	targets, err := d.resolveDialTargets(ctx, addr)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, target := range targets {
		conn, err := d.dialer.DialContext(ctx, network, target)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no dial targets resolved for %s", addr)
}

func (d *policyDialer) resolveDialTargets(ctx context.Context, addr string) ([]string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if port == "" {
		return nil, fmt.Errorf("missing port for %s", addr)
	}
	host = stripHostBrackets(host)
	if d.proxy != nil {
		if targets, matched, err := d.proxy.resolveAllowedProxyDialTargets(ctx, host, port); matched || err != nil {
			return targets, err
		}
	}
	if ip, ok := parseIPLikeHost(host); ok {
		if isBlockedIP(ip, d.urlPolicy.cfg) {
			return nil, fmt.Errorf("dial target resolves to private or reserved IP (%s)", ip.String())
		}
		return []string{net.JoinHostPort(ip.String(), port)}, nil
	}
	if err := d.urlPolicy.Check(ctx, (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, port)}).String(), PurposeRedirect); err != nil {
		return nil, err
	}
	addrs, err := d.urlPolicy.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		if d.urlPolicy.cfg.DNSFailMode == "open_dev_only" {
			return []string{addr}, nil
		}
		return nil, fmt.Errorf("dial target %q DNS resolution failed", host)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("dial target %q DNS returned no addresses", host)
	}
	targets := make([]string, 0, len(addrs))
	for _, resolved := range addrs {
		if isBlockedIP(resolved.IP, d.urlPolicy.cfg) {
			return nil, fmt.Errorf("dial target %q resolves to private or reserved IP (%s)", host, resolved.IP.String())
		}
		ip := resolved.IP.String()
		if resolved.Zone != "" {
			ip = ip + "%" + resolved.Zone
		}
		targets = append(targets, net.JoinHostPort(ip, port))
	}
	return targets, nil
}

func stripHostBrackets(host string) string {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		return host[1 : len(host)-1]
	}
	return host
}
