package fileop

import "testing"

func TestValidateBoundary(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		base    string
		wantErr bool
	}{
		{"inside", "/workspace/foo/bar.txt", "/workspace", false},
		{"exact boundary", "/workspace", "/workspace", false},
		{"outside", "/tmp/evil.txt", "/workspace", true},
		{"traversal escape", "/workspace/../etc/passwd", "/workspace", true},
		{"relative inside", "sub/file.txt", "/workspace", false},
		{"empty base skips", "/anywhere/file.txt", "", false},
		{"prefix attack", "/workspace-evil/file.txt", "/workspace", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBoundary(tt.path, tt.base)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBoundary(%q, %q) error = %v, wantErr %v", tt.path, tt.base, err, tt.wantErr)
			}
		})
	}
}

func TestContainsTraversal(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"normal/path/file.txt", false},
		{"../escape", true},
		{"foo/../../bar", true},
		{"foo/bar", false},
		{`foo\..\bar`, true},
		{"foo/..", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := ContainsTraversal(tt.path); got != tt.want {
				t.Errorf("ContainsTraversal(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestResolveRelative(t *testing.T) {
	if got := ResolveRelative("/abs/path", "/base"); got != "/abs/path" {
		t.Errorf("absolute path should be unchanged, got %q", got)
	}
	if got := ResolveRelative("rel/path", "/base"); got != "/base/rel/path" {
		t.Errorf("relative path should resolve, got %q", got)
	}
}
