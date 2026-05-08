package permission

import (
	"testing"
)

func TestMatchFileRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		path    string
		want    bool
	}{
		// Deny: sensitive files
		{"env file", Rule{Tool: "*", Pattern: ".env"}, ".env", true},
		{"env file in subdir", Rule{Tool: "*", Pattern: ".env"}, "config/.env", true},
		{"env variant", Rule{Tool: "*", Pattern: ".env.*"}, ".env.production", true},
		{"pem file deep", Rule{Tool: "*", Pattern: "**/*.pem"}, "certs/server.pem", true},
		{"key file", Rule{Tool: "*", Pattern: "**/*.key"}, "ssl/private.key", true},
		{"credential file", Rule{Tool: "*", Pattern: "**/*credential*"}, "config/credentials.json", true},
		{"secrets dir", Rule{Tool: "*", Pattern: "**/secrets/**"}, "secrets/api.txt", true},
		{"id_rsa", Rule{Tool: "*", Pattern: "**/*id_rsa*"}, ".ssh/id_rsa", true},

		// Allow: normal files
		{"go file", Rule{Tool: "Edit", Pattern: "**"}, "internal/app/app.go", true},
		{"any file", Rule{Tool: "View", Pattern: "**"}, "README.md", true},

		// No match
		{"non-env", Rule{Tool: "*", Pattern: ".env"}, "config.json", false},
		{"non-pem", Rule{Tool: "*", Pattern: "**/*.pem"}, "cert.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchFileRule(tt.rule, tt.path)
			if got != tt.want {
				t.Errorf("MatchFileRule(%q, %q) = %v, want %v", tt.rule.Pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchFileRule_CaseInsensitive(t *testing.T) {
	// On darwin/windows, MatchFileRule should match regardless of case.
	// On linux, the test verifies paths are matched correctly as-is (both lowercased).
	tests := []struct {
		name string
		rule Rule
		path string
		want bool
	}{
		{"uppercase env", Rule{Tool: "*", Pattern: ".env"}, ".ENV", true},
		{"mixed case pem", Rule{Tool: "*", Pattern: "**/*.pem"}, "Certs/Server.PEM", true},
		{"uppercase key", Rule{Tool: "*", Pattern: "**/*.key"}, "SSL/PRIVATE.KEY", true},
		{"mixed credential", Rule{Tool: "*", Pattern: "**/*credential*"}, "Config/MyCredentials.json", true},
		{"uppercase go file allow", Rule{Tool: "Edit", Pattern: "**"}, "Internal/App/App.Go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchFileRule(tt.rule, tt.path)
			if got != tt.want {
				t.Errorf("MatchFileRule(%q, %q) = %v, want %v", tt.rule.Pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchBashRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		command string
		want    bool
	}{
		// Word boundary: "ls *" matches "ls -la" but not "lsof"
		{"ls with args", Rule{Tool: "Bash", Pattern: "ls *", Decision: Allow}, "ls -la", true},
		{"ls bare", Rule{Tool: "Bash", Pattern: "ls *", Decision: Allow}, "ls", true},
		{"lsof no match", Rule{Tool: "Bash", Pattern: "ls *", Decision: Allow}, "lsof", false},

		// Exact match
		{"pwd exact", Rule{Tool: "Bash", Pattern: "pwd", Decision: Allow}, "pwd", true},
		{"pwd with args", Rule{Tool: "Bash", Pattern: "pwd", Decision: Allow}, "pwd -L", false},

		// Deny: dangerous commands
		{"rm -rf", Rule{Tool: "Bash", Pattern: "rm -rf *", Decision: Deny}, "rm -rf /tmp/data", true},
		{"rm -rf bare", Rule{Tool: "Bash", Pattern: "rm -rf *", Decision: Deny}, "rm -rf", true},
		{"rm safe", Rule{Tool: "Bash", Pattern: "rm -rf *", Decision: Deny}, "rm file.txt", false},
		{"git push force", Rule{Tool: "Bash", Pattern: "git push --force *", Decision: Deny}, "git push --force origin main", true},
		{"git push normal", Rule{Tool: "Bash", Pattern: "git push --force *", Decision: Deny}, "git push origin main", false},
		{"chmod 777", Rule{Tool: "Bash", Pattern: "chmod 777 *", Decision: Deny}, "chmod 777 /tmp", true},

		// Allow: git commands
		{"git status", Rule{Tool: "Bash", Pattern: "git status", Decision: Allow}, "git status", true},
		{"git status args", Rule{Tool: "Bash", Pattern: "git status *", Decision: Allow}, "git status --short", true},
		{"git log", Rule{Tool: "Bash", Pattern: "git log *", Decision: Allow}, "git log --oneline -5", true},

		// Allow: Go toolchain
		{"go test", Rule{Tool: "Bash", Pattern: "go test *", Decision: Allow}, "go test ./internal/...", true},
		{"go build", Rule{Tool: "Bash", Pattern: "go build *", Decision: Allow}, "go build -o bin/app", true},
		{"go version", Rule{Tool: "Bash", Pattern: "go version", Decision: Allow}, "go version", true},

		// Allow: build tools
		{"make", Rule{Tool: "Bash", Pattern: "make *", Decision: Allow}, "make build", true},
		{"make bare", Rule{Tool: "Bash", Pattern: "make", Decision: Allow}, "make", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchBashRule(tt.rule, tt.command)
			if got != tt.want {
				t.Errorf("MatchBashRule(%q, %q) = %v, want %v", tt.rule.Pattern, tt.command, got, tt.want)
			}
		})
	}
}

func TestMatchBashRule_CompoundCommands(t *testing.T) {
	denyRmRf := Rule{Tool: "Bash", Pattern: "rm -rf *", Decision: Deny}
	denyForce := Rule{Tool: "Bash", Pattern: "git push --force *", Decision: Deny}

	tests := []struct {
		name    string
		rule    Rule
		command string
		want    bool
	}{
		{"safe && dangerous", denyRmRf, "ls && rm -rf /", true},
		{"dangerous && safe", denyRmRf, "rm -rf / && ls", true},
		{"safe ; dangerous", denyRmRf, "echo hello ; rm -rf /", true},
		{"safe || dangerous", denyRmRf, "test -f x || rm -rf /", true},
		{"pipe to dangerous", denyRmRf, "find . | rm -rf /tmp", true},
		{"all safe", denyRmRf, "ls && pwd && echo hello", false},
		{"force push in compound", denyForce, "git add . && git push --force origin main", true},
		// Quoted operators should NOT split
		{"quoted &&", denyRmRf, `echo "a && b"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchBashRule(tt.rule, tt.command)
			if got != tt.want {
				t.Errorf("MatchBashRule(%q, %q) = %v, want %v", tt.rule.Pattern, tt.command, got, tt.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	rules := append(DefaultDenyRules(), DefaultAskRules()...)
	rules = append(rules, DefaultAllowRules()...)

	tests := []struct {
		name     string
		toolName string
		target   string
		want     Decision
	}{
		// Deny wins over allow
		{"deny .env", "Edit", ".env", Deny},
		{"deny .env.prod", "Write", ".env.production", Deny},
		{"deny pem", "View", "certs/server.pem", Deny},
		{"deny rm -rf", "Bash", "rm -rf /tmp", Deny},
		{"deny git force push", "Bash", "git push --force origin main", Deny},
		{"deny git reset hard", "Bash", "git reset --hard HEAD~1", Deny},

		// Allow for safe operations
		{"allow go file edit", "Edit", "internal/app/app.go", Allow},
		{"allow view", "View", "README.md", Allow},
		{"allow git status", "Bash", "git status", Allow},
		{"allow go test", "Bash", "go test ./...", Allow},
		{"allow ls", "Bash", "ls -la", Allow},
		{"allow make", "Bash", "make build", Allow},
		{"allow make bare", "Bash", "make", Allow},

		// Ask for dangerous patterns (from DefaultAskRules)
		{"ask python -c", "Bash", "python -c 'import os'", Ask},
		{"ask python3 script", "Bash", "python3 script.py", Ask},
		{"ask curl", "Bash", "curl https://example.com", Ask},
		{"ask wget", "Bash", "wget https://example.com", Ask},
		{"ask sudo", "Bash", "sudo rm -rf /tmp", Ask},
		{"ask kubectl", "Bash", "kubectl get pods", Ask},
		{"ask aws", "Bash", "aws s3 ls", Ask},
		{"ask ssh", "Bash", "ssh user@host", Ask},
		{"ask gcloud", "Bash", "gcloud compute instances list", Ask},
		{"ask terraform", "Bash", "terraform apply", Ask},

		// Ask for unknown commands
		{"ask unknown", "Bash", "some-unknown-command", Ask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(rules, tt.toolName, tt.target)
			if got != tt.want {
				t.Errorf("Evaluate(%q, %q) = %v, want %v", tt.toolName, tt.target, got, tt.want)
			}
		})
	}
}

func TestSplitCompoundCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want int // expected number of parts
	}{
		{"simple", "ls", 1},
		{"and", "ls && pwd", 2},
		{"or", "test -f x || echo no", 2},
		{"semicolon", "ls; pwd; date", 3},
		{"pipe", "cat file | grep pattern", 2},
		{"mixed", "ls && echo ok ; date", 3},
		{"quoted and", `echo "a && b"`, 1},
		{"quoted semicolon", `echo 'a;b'`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitCompoundCommand(tt.cmd)
			if len(parts) != tt.want {
				t.Errorf("splitCompoundCommand(%q) got %d parts %v, want %d", tt.cmd, len(parts), parts, tt.want)
			}
		})
	}
}

func TestMakeRelativePath(t *testing.T) {
	tests := []struct {
		name      string
		absPath   string
		workspace string
		want      string
	}{
		{"inside workspace", "/home/user/project/src/main.go", "/home/user/project", "src/main.go"},
		{"workspace root", "/home/user/project", "/home/user/project", "."},
		{"outside workspace", "/etc/passwd", "/home/user/project", "/etc/passwd"},
		{"relative path", "src/main.go", "/home/user/project", "src/main.go"},
		{"empty workspace", "/home/user/project/src/main.go", "", "/home/user/project/src/main.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeRelativePath(tt.absPath, tt.workspace)
			if got != tt.want {
				t.Errorf("MakeRelativePath(%q, %q) = %q, want %q", tt.absPath, tt.workspace, got, tt.want)
			}
		})
	}
}
