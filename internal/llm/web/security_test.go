package web

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeResolver struct {
	ips map[string][]net.IPAddr
	err map[string]error
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if err := f.err[host]; err != nil {
		return nil, err
	}
	if v, ok := f.ips[host]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func TestValidateWebURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid public ipv4", "https://1.1.1.1/path", false},
		{"valid public ipv6", "https://[2606:4700:4700::1111]/path", false},
		{"ftp scheme", "ftp://example.com", true},
		{"userinfo blocked", "https://u:p@example.com", true},
		{"localhost", "https://localhost/api", true},
		{"single-label host", "https://intranet/page", true},
		{"private ipv4", "https://10.0.0.1/internal", true},
		{"reserved documentation ipv4", "https://192.0.2.1/a", true},
		{"reserved documentation ipv6", "https://[2001:db8::1]/a", true},
		{"deprecated ipv6 site local", "https://[fec0::1]/a", true},
		{"teredo ipv6", "https://[2001::1]/a", true},
		{"6to4 ipv6", "https://[2002::1]/a", true},
		{"deprecated 6to4 relay ipv4", "https://192.88.99.1/a", true},
		{"link local ipv4", "https://169.254.1.1/a", true},
		{"multicast ipv4", "https://224.0.0.1/a", true},
		{"unspecified ipv4", "https://0.0.0.1/a", true},
		{"ipv6 ula", "https://[fd00::1]/a", true},
		{"ipv4 mapped ipv6 private", "https://[::ffff:127.0.0.1]/a", true},
		{"cgnat", "https://100.64.0.1/a", true},
		{"metadata ip", "https://169.254.169.254/latest", true},
		{"metadata host", "https://metadata.google.internal/computeMetadata", true},
		{"dword obfuscated", "https://2130706433/a", true},
		{"hex obfuscated", "https://0x7f000001/a", true},
		{"octal obfuscated", "https://0177.0.0.1/a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWebURL(%q) error = %v, wantErr = %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestURLPolicy_DNSFailClosed(t *testing.T) {
	p := NewURLPolicy(DefaultConfig().URLPolicy).withResolver(&fakeResolver{err: map[string]error{"example.com": errors.New("dns fail")}})
	if err := p.Check(context.Background(), "https://example.com/a", PurposeFetch); err == nil {
		t.Fatal("expected dns fail-closed error")
	}
}

func TestURLPolicy_DNSFailOpenDevOnly(t *testing.T) {
	cfg := DefaultConfig().URLPolicy
	cfg.DNSFailMode = "open_dev_only"
	p := NewURLPolicy(cfg).withResolver(&fakeResolver{err: map[string]error{"example.com": errors.New("dns fail")}})
	if err := p.Check(context.Background(), "https://example.com/a", PurposeFetch); err != nil {
		t.Fatalf("expected allow in open_dev_only mode, got %v", err)
	}
}

func TestIsPermittedRedirect(t *testing.T) {
	tests := []struct {
		name     string
		original string
		redirect string
		want     bool
	}{
		{"same host path change", "https://example.com/a", "https://example.com/b", true},
		{"add www", "https://example.com/a", "https://www.example.com/a", true},
		{"remove www", "https://www.example.com/a", "https://example.com/a", true},
		{"cross host", "https://example.com/a", "https://evil.com/a", false},
		{"protocol change", "https://example.com/a", "http://example.com/a", false},
		{"port change", "https://example.com:443/a", "https://example.com:8080/a", false},
		{"redirect with userinfo", "https://example.com/a", "https://user@example.com/a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPermittedRedirect(tt.original, tt.redirect)
			if got != tt.want {
				t.Errorf("IsPermittedRedirect(%q, %q) = %v, want %v", tt.original, tt.redirect, got, tt.want)
			}
		})
	}
}
