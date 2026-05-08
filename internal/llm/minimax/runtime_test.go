package minimax

import "testing"

func TestResolveRuntimeDefaultsToTokenPlanBearer(t *testing.T) {
	rt, err := ResolveRuntime("", "", "")
	if err != nil {
		t.Fatalf("ResolveRuntime error: %v", err)
	}
	if rt.BaseURL != TokenPlanBaseURL {
		t.Fatalf("baseURL=%q want %q", rt.BaseURL, TokenPlanBaseURL)
	}
	if rt.Auth != AuthBearer {
		t.Fatalf("auth=%q want %q", rt.Auth, AuthBearer)
	}
}

func TestResolveRuntimeProfileRules(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		profile string
		auth    string
		wantURL string
		want    AuthMode
	}{
		{name: "token-plan", profile: "token-plan", wantURL: TokenPlanBaseURL, want: AuthBearer},
		{name: "legacy", profile: "legacy", wantURL: LegacyBaseURL, want: AuthXAPIKey},
		{name: "current official base explicit", baseURL: "https://api.minimaxi.com/anthropic", profile: "auto", wantURL: "https://api.minimaxi.com/anthropic", want: AuthBearer},
		{name: "previous token base explicit", baseURL: "https://api.minimax.io/anthropic", profile: "auto", wantURL: "https://api.minimax.io/anthropic", want: AuthBearer},
		{name: "custom defaults xapi", baseURL: "https://example.com/anthropic", profile: "custom", wantURL: "https://example.com/anthropic", want: AuthXAPIKey},
		{name: "explicit auth wins", baseURL: "https://api.minimaxi.com/anthropic", profile: "auto", auth: "anthropic_x_api_key", wantURL: "https://api.minimaxi.com/anthropic", want: AuthXAPIKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := ResolveRuntime(tc.baseURL, tc.profile, tc.auth)
			if err != nil {
				t.Fatalf("ResolveRuntime error: %v", err)
			}
			if rt.BaseURL != tc.wantURL {
				t.Fatalf("baseURL=%q want %q", rt.BaseURL, tc.wantURL)
			}
			if rt.Auth != tc.want {
				t.Fatalf("auth=%q want %q", rt.Auth, tc.want)
			}
		})
	}
}

func TestResolveRuntimeErrors(t *testing.T) {
	if _, err := ResolveRuntime("", "custom", ""); err == nil {
		t.Fatal("expected custom without baseURL error")
	}
	if _, err := ResolveRuntime("", "mystery", ""); err == nil {
		t.Fatal("expected unsupported profile error")
	}
	if _, err := ResolveRuntime("", "", "mystery"); err == nil {
		t.Fatal("expected unsupported auth mode error")
	}
}
