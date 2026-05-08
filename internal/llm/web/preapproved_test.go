package web

import "testing"

func TestIsPreapprovedHost(t *testing.T) {
	tests := []struct {
		hostname string
		pathname string
		want     bool
	}{
		// CC original domains
		{"docs.python.org", "/3/library/os.html", true},
		{"developer.mozilla.org", "/en-US/docs/Web/API", true},
		{"go.dev", "/doc/effective_go", true},
		{"pkg.go.dev", "/fmt", true},
		{"react.dev", "/learn", true},
		{"www.sqlite.org", "/index.html", true},
		{"kubernetes.io", "/docs/concepts", true},
		{"git-scm.com", "/book/en/v2", true},

		// Path-scoped: github.com/anthropics
		{"github.com", "/anthropics", true},
		{"github.com", "/anthropics/claude-code", true},
		{"github.com", "/anthropics-evil/malware", false}, // must not match
		{"github.com", "/someone/else", false},

		// OpenScholar academic additions
		{"arxiv.org", "/abs/2401.00001", true},
		{"scholar.google.com", "/scholar?q=test", true},
		{"openreview.net", "/forum?id=abc", true},
		{"aclanthology.org", "/2024.acl-long.1", true},
		{"paperswithcode.com", "/sota/image-classification", true},

		// Not pre-approved
		{"evil.com", "/", false},
		{"example.com", "/page", false},
		{"google.com", "/search", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname+tt.pathname, func(t *testing.T) {
			got := IsPreapprovedHost(tt.hostname, tt.pathname)
			if got != tt.want {
				t.Errorf("IsPreapprovedHost(%q, %q) = %v, want %v", tt.hostname, tt.pathname, got, tt.want)
			}
		})
	}
}
