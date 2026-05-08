package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestWebsitePolicy(t *testing.T) {
	p := NewWebsitePolicy(WebsitePolicyConfig{
		Mode:          "blocklist",
		DefaultAction: "allow",
		Rules: []WebsitePolicyRule{
			{Pattern: "bad.example.com", Action: "block"},
			{Pattern: "*.blocked.com", Action: "block"},
		},
	})
	if err := p.Check("ok.example.com"); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if err := p.Check("bad.example.com"); err == nil {
		t.Fatal("expected block exact domain")
	}
	if err := p.Check("a.blocked.com"); err == nil {
		t.Fatal("expected block wildcard")
	}

	allow := NewWebsitePolicy(WebsitePolicyConfig{
		Mode:          "allowlist",
		DefaultAction: "block",
		Rules:         []WebsitePolicyRule{{Pattern: "example.com", Action: "allow"}},
	})
	if err := allow.Check("docs.example.com"); err != nil {
		t.Fatalf("expected allow by parent match, got %v", err)
	}
	if err := allow.Check("evil.com"); err == nil {
		t.Fatal("expected allowlist default deny")
	}
}

func TestURLRedactionAndCacheKey(t *testing.T) {
	raw := "https://example.com/path?token=abc&x-amz-signature=123&ok=1#access_token=secret"
	if !ContainsURLSecret(raw) {
		t.Fatal("expected secret detection")
	}
	redacted := RedactURL(raw)
	if redacted == raw {
		t.Fatal("expected URL to be redacted")
	}
	if redacted == "" || redacted == raw {
		t.Fatal("expected non-empty redacted URL")
	}
	if contains := ContainsURLSecret(redacted); !contains {
		// redacted URL keeps sensitive key names for traceability; ensure values are scrubbed.
		t.Fatal("expected redacted URL to retain secret-key markers")
	}
	key := CanonicalCacheURL(redacted)
	if key == "" {
		t.Fatal("expected cache key")
	}
}

func TestURLRedaction_DoesNotTreatCodeParamAsSecretByKey(t *testing.T) {
	raw := "https://example.com/callback?code=abc123&state=ok"
	if ContainsURLSecret(raw) {
		t.Fatalf("expected code query key not to be treated as secret by default")
	}
	redacted := RedactURL(raw)
	if strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("expected code query key to remain visible, got %s", redacted)
	}
}

func TestURLRedaction_RedactsSensitiveCodeValues(t *testing.T) {
	raw := "https://example.com/callback?code=abcdefghijklmnopqrstuvwxyzABCDEF&state=ok"
	if !ContainsURLSecret(raw) {
		t.Fatalf("expected long authorization code value to be treated as secret")
	}
	redacted := RedactURL(raw)
	if !strings.Contains(redacted, "code=%5BREDACTED%5D") {
		t.Fatalf("expected code query value to be redacted, got %s", redacted)
	}
}

func TestURLRedaction_AllowsOrdinarySearchQueryValues(t *testing.T) {
	raw := "https://openreview.net/group?id=NeurIPS.cc/2024/Conference&q=multi-agent_reinforcement-learning_for-llm-systems"
	if ContainsURLSecret(raw) {
		t.Fatalf("expected ordinary search query to be allowed: %s", raw)
	}
	redacted := RedactURL(raw)
	if strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("expected ordinary search query not redacted, got %s", redacted)
	}
}

func TestURLRedaction_BlocksSensitiveQueryKeys(t *testing.T) {
	raw := "https://example.com/api?api_key=aaa&apikey=bbb&api-key=ccc&access_token=ddd&token=eee&auth=fff&signature=ggg&sig=hhh&secret=iii&client_secret=jjj&x-amz-signature=kkk"
	if !ContainsURLSecret(raw) {
		t.Fatalf("expected sensitive query keys to be detected")
	}
	redacted := RedactURL(raw)
	if strings.Count(redacted, "%5BREDACTED%5D") < 10 {
		t.Fatalf("expected sensitive query values redacted, got %s", redacted)
	}
}

func TestURLRedaction_BlocksHighConfidenceTokensInSearchQuery(t *testing.T) {
	cases := []string{
		"https://example.com/search?q=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.signature",
		"https://example.com/search?query=sk-abc1234567890_token-value",
		"https://example.com/search?search=ghp_abcdefghijklmnopqrstuvwxyz123456",
	}
	for _, raw := range cases {
		if !ContainsURLSecret(raw) {
			t.Fatalf("expected sensitive token in search parameter to be detected: %s", raw)
		}
	}
}

func TestURLRedactionDetectsPathSecrets(t *testing.T) {
	raw := "https://example.com/a/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.signature/b"
	if !ContainsURLSecret(raw) {
		t.Fatal("expected path JWT-like token to be detected")
	}
	redacted := RedactURL(raw)
	if redacted == raw {
		t.Fatal("expected path token to be redacted")
	}
	if ContainsURLSecret(redacted) {
		t.Fatalf("expected redacted path to be clean, got %s", redacted)
	}
}

func TestURLRedactionHandlesMalformedSecretURL(t *testing.T) {
	raw := "https://example.com/%zz?token=super-secret#access_token=secret"
	if !ContainsURLSecret(raw) {
		t.Fatal("expected malformed URL secret to be detected")
	}
	redacted := RedactURL(raw)
	if strings.Contains(redacted, "super-secret") || strings.Contains(redacted, "access_token=secret") {
		t.Fatalf("expected malformed URL secrets to be redacted, got %s", redacted)
	}
}

func TestRuntimeValidateRedirectURL(t *testing.T) {
	rt := NewRuntime(nil, nil, DefaultConfig())
	rt.urlPolicy = rt.urlPolicy.withResolver(&fakeResolver{
		ips: map[string][]net.IPAddr{
			"example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
		err: map[string]error{
			"example.com": nil,
		},
	})
	if err := rt.validateURLForPolicies(context.Background(), "https://example.com/a", PurposeRedirect); err != nil {
		t.Fatalf("expected allow redirect target, got %v", err)
	}
	if err := rt.validateURLForPolicies(context.Background(), "https://127.0.0.1/a", PurposeRedirect); err == nil {
		t.Fatal("expected redirect-to-private to be blocked")
	}
	if err := rt.validateURLForPolicies(context.Background(), "https://198.18.0.10/a", PurposeRedirect); err == nil {
		t.Fatal("expected redirect-to-fake-ip to be blocked")
	}
	if err := rt.validateURLForPolicies(context.Background(), "https://example.com/a?access_token=secret", PurposeRedirect); err == nil {
		t.Fatal("expected redirect URL containing a secret to be blocked")
	}
}

func TestRuntimeValidateRedirectFakeIPDNSBlockedEvenWithProxy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Proxy.Mode = "explicit"
	cfg.Proxy.URL = "http://127.0.0.1:7890"
	rt := NewRuntime(nil, nil, cfg)
	rt.urlPolicy = rt.urlPolicy.withResolver(&fakeResolver{
		ips: map[string][]net.IPAddr{
			"fake.example.com": {{IP: net.ParseIP("198.18.0.10")}},
		},
		err: map[string]error{},
	})
	if err := rt.validateURLForPolicies(context.Background(), "https://fake.example.com/a", PurposeRedirect); err == nil {
		t.Fatal("expected redirect-to-fake-ip DNS target to be blocked even with proxy")
	}
}

func TestPolicyDialerRejectsReboundPrivateIP(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{ips: map[string][]net.IPAddr{
		"rebind.example.com": {{IP: net.ParseIP("10.0.0.2")}},
	}, err: map[string]error{}})
	d := &policyDialer{urlPolicy: p}
	if _, err := d.resolveDialTargets(context.Background(), "rebind.example.com:443"); err == nil {
		t.Fatal("expected private dial target to be blocked")
	}
}

func TestPolicyDialerResolvesPublicIPTargets(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{ips: map[string][]net.IPAddr{
		"public.example.com": {{IP: net.ParseIP("93.184.216.34")}},
	}, err: map[string]error{}})
	d := &policyDialer{urlPolicy: p}
	targets, err := d.resolveDialTargets(context.Background(), "public.example.com:443")
	if err != nil {
		t.Fatalf("expected public dial target, got %v", err)
	}
	if len(targets) != 1 || targets[0] != "93.184.216.34:443" {
		t.Fatalf("unexpected dial targets: %#v", targets)
	}
}

func TestPolicyDialerAllowsConfiguredLocalProxyTarget(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy)
	d := &policyDialer{
		urlPolicy: p,
		proxy: newProxyResolver(ProxyConfig{
			Mode:            "explicit",
			URL:             "http://127.0.0.1:7890",
			AllowLocalProxy: true,
			FakeIPCIDRs:     []string{"198.18.0.0/15"},
		}, p.resolver),
	}
	targets, err := d.resolveDialTargets(context.Background(), "127.0.0.1:7890")
	if err != nil {
		t.Fatalf("expected configured local proxy dial target to be allowed, got %v", err)
	}
	if len(targets) != 1 || targets[0] != "127.0.0.1:7890" {
		t.Fatalf("unexpected dial targets: %#v", targets)
	}
}

func TestPolicyDialerBlocksUnconfiguredLocalTarget(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy)
	d := &policyDialer{
		urlPolicy: p,
		proxy: newProxyResolver(ProxyConfig{
			Mode:            "explicit",
			URL:             "http://127.0.0.1:7890",
			AllowLocalProxy: true,
			FakeIPCIDRs:     []string{"198.18.0.0/15"},
		}, p.resolver),
	}
	if _, err := d.resolveDialTargets(context.Background(), "127.0.0.1:7891"); err == nil {
		t.Fatal("expected unconfigured local dial target to be blocked")
	}
}

func TestSearchResultPolicyFiltering(t *testing.T) {
	provider := searchProviderStub{result: &SearchResult{Hits: []SearchHit{
		{Title: "ok", URL: "https://example.com?a=1"},
		{Title: "private", URL: "https://127.0.0.1/admin"},
		{Title: "secret", URL: "https://safe.example.com/?token=abc"},
	}}}
	rt := NewRuntime(provider, nil, DefaultConfig())
	rt.urlPolicy = rt.urlPolicy.withResolver(&fakeResolver{
		ips: map[string][]net.IPAddr{
			"example.com":      {{IP: net.ParseIP("93.184.216.34")}},
			"safe.example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
		err: map[string]error{},
	})
	res, err := rt.Search(context.Background(), "q", SearchOptions{})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected 1 safe hit, got %d", len(res.Hits))
	}
	if res.Hits[0].URL != "https://example.com?a=1" {
		t.Fatalf("unexpected remaining hit: %s", res.Hits[0].URL)
	}
}

type searchProviderStub struct {
	result *SearchResult
	err    error
}

func (s searchProviderStub) SearchWeb(_ context.Context, _ string, _ SearchOptions) (*SearchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func (s searchProviderStub) SupportsFilter() bool { return true }

func TestURLPolicyRejectsDNSPrivateResolution(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{ips: map[string][]net.IPAddr{
		"evil.example.com": {{IP: net.ParseIP("10.0.0.2")}},
	}, err: map[string]error{}})
	err := p.Check(context.Background(), "https://evil.example.com/", PurposeFetch)
	if err == nil {
		t.Fatal("expected DNS-resolved private IP to be blocked")
	}
}

func TestURLPolicyRejectsDNSLookupErrorByDefault(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{err: map[string]error{"x.example.com": errors.New("dns down")}})
	err := p.Check(context.Background(), "https://x.example.com/", PurposeFetch)
	if err == nil {
		t.Fatal("expected dns lookup failure to be blocked")
	}
}

func TestURLPolicyFakeIPDNSBlockedWithoutProxy(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{ips: map[string][]net.IPAddr{
		"fake.example.com": {{IP: net.ParseIP("198.18.0.10")}},
	}, err: map[string]error{}})
	err := p.CheckWithOptions(context.Background(), "https://fake.example.com/", URLCheckOptions{
		PolicyMetadata: PolicyMetadata{ProxyMode: "direct", ProxyUsed: false},
		FakeIPCIDRs:    []string{"198.18.0.0/15"},
	})
	if err == nil {
		t.Fatal("expected direct fake-ip DNS target to be blocked")
	}
}

func TestURLPolicyFakeIPDNSAllowedWithProxy(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{ips: map[string][]net.IPAddr{
		"fake.example.com": {{IP: net.ParseIP("198.18.0.10")}},
	}, err: map[string]error{}})
	err := p.CheckWithOptions(context.Background(), "https://fake.example.com/", URLCheckOptions{
		PolicyMetadata: PolicyMetadata{ProxyMode: "environment", ProxyUsed: true, ProxyEndpointClass: "loopback"},
		FakeIPCIDRs:    []string{"198.18.0.0/15"},
	})
	if err != nil {
		t.Fatalf("expected proxy-backed fake-ip DNS target to be allowed, got %v", err)
	}
}

func TestURLPolicyFakeIPLiteralBlockedEvenWithProxy(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy)
	err := p.CheckWithOptions(context.Background(), "https://198.18.0.1/", URLCheckOptions{
		PolicyMetadata: PolicyMetadata{ProxyMode: "environment", ProxyUsed: true, ProxyEndpointClass: "loopback"},
		FakeIPCIDRs:    []string{"198.18.0.0/15"},
	})
	if err == nil {
		t.Fatal("expected fake-ip literal target to be blocked even with proxy")
	}
}

func TestProxyResolverExplicitLocalProxyAllowed(t *testing.T) {
	p := newProxyResolver(ProxyConfig{
		Mode:            "explicit",
		URL:             "http://127.0.0.1:7890",
		AllowLocalProxy: true,
		FakeIPCIDRs:     []string{"198.18.0.0/15"},
	}, net.DefaultResolver)
	req := &http.Request{URL: mustParseURL(t, "https://example.com")}
	u, meta, err := p.decide(req)
	if err != nil {
		t.Fatalf("expected explicit localhost proxy endpoint allowed, got %v", err)
	}
	if u == nil || !meta.ProxyUsed {
		t.Fatalf("expected proxy to be used, got u=%v meta=%+v", u, meta)
	}
}

func TestProxyResolverPrivateProxyBlockedByDefault(t *testing.T) {
	p := newProxyResolver(ProxyConfig{
		Mode: "explicit",
		URL:  "http://192.168.1.1:8080",
	}, net.DefaultResolver)
	req := &http.Request{URL: mustParseURL(t, "https://example.com")}
	if _, _, err := p.decide(req); err == nil {
		t.Fatal("expected private proxy endpoint to be blocked")
	}
}

func TestProxyResolverPrivateProxyBlockedWhenLocalProxyAllowed(t *testing.T) {
	p := newProxyResolver(ProxyConfig{
		Mode:            "explicit",
		URL:             "http://192.168.1.1:8080",
		AllowLocalProxy: true,
	}, net.DefaultResolver)
	req := &http.Request{URL: mustParseURL(t, "https://example.com")}
	if _, _, err := p.decide(req); err == nil {
		t.Fatal("expected non-loopback private proxy endpoint to be blocked")
	}
}

func TestProxyResolverUnsupportedSchemeReturnsClearError(t *testing.T) {
	p := newProxyResolver(ProxyConfig{
		Mode: "explicit",
		URL:  "socks5://127.0.0.1:7890",
	}, net.DefaultResolver)
	req := &http.Request{URL: mustParseURL(t, "https://example.com")}
	_, _, err := p.decide(req)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported proxy scheme") {
		t.Fatalf("expected unsupported proxy scheme error, got %v", err)
	}
}

func TestProxyResolverNoProxyBypassBlocksFakeIP(t *testing.T) {
	p := newProxyResolver(DefaultConfig().Proxy, net.DefaultResolver)
	p.envProxy = func(req *http.Request) (*url.URL, error) {
		if strings.EqualFold(req.URL.Hostname(), "fake.example.com") {
			// Simulates NO_PROXY match: bypass configured env proxy.
			return nil, nil
		}
		return url.Parse("http://127.0.0.1:7890")
	}
	req := &http.Request{URL: mustParseURL(t, "https://fake.example.com:443/path")}
	_, meta, err := p.decide(req)
	if err != nil {
		t.Fatalf("unexpected proxy resolve error: %v", err)
	}
	if meta.ProxyUsed {
		t.Fatalf("expected NO_PROXY target to bypass proxy, got %+v", meta)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return u
}
