package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathSecurity(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		writeOp bool
		wantErr bool
	}{
		// 应拒绝
		{"dollar_expansion", "$(cat /etc/passwd)", false, true},
		{"backtick_expansion", "`whoami`", false, true},
		{"env_var", "$HOME/.bashrc", false, true},
		{"percent_var", "%APPDATA%\\config", false, true},
		{"unc_backslash", `\\server\share\file`, false, true},
		{"unc_forward", "//server/share/file", false, true},
		{"tilde_user", "~root/.ssh/id_rsa", false, true},
		{"tilde_plus", "~+/foo", false, true},
		{"write_glob_star", "/src/*.go", true, true},
		{"write_glob_question", "/src/?.go", true, true},
		{"write_glob_bracket", "/src/[ab].go", true, true},
		// 应放行
		{"normal_absolute", "/tmp/test.go", false, false},
		{"normal_relative", "src/main.go", false, false},
		{"tilde_home", "~/project/main.go", false, false},
		{"tilde_alone", "~", false, false},
		{"read_glob_ok", "/src/*.go", false, false}, // 读操作允许 glob
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSecurity(tt.path, tt.writeOp)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePathSecurity(%q, %v) error = %v, wantErr %v", tt.path, tt.writeOp, err, tt.wantErr)
			}
		})
	}
}

func TestResolveAndValidatePath(t *testing.T) {
	// 创建临时工作区
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "workspace")
	os.MkdirAll(workDir, 0o755)

	// 工作区内的文件应该通过
	innerFile := filepath.Join(workDir, "test.go")
	os.WriteFile(innerFile, []byte("test"), 0o644)
	resolved, err := resolveAndValidatePath(innerFile, workDir)
	if err != nil {
		t.Errorf("expected success for workspace file, got: %v", err)
	}
	if resolved == "" {
		t.Error("resolved path should not be empty")
	}

	// 符号链接逃逸应该被拒绝
	os.Symlink("/etc/passwd", filepath.Join(workDir, "link"))
	_, err = resolveAndValidatePath(filepath.Join(workDir, "link"), workDir)
	if err == nil {
		t.Error("expected error for symlink escaping workspace")
	}

	// 新文件（不存在）应该通过（在工作区内）
	newFile := filepath.Join(workDir, "new.go")
	_, err = resolveAndValidatePath(newFile, workDir)
	if err != nil {
		t.Errorf("expected success for new file in workspace, got: %v", err)
	}
}

func TestIsDangerousRemovalTarget(t *testing.T) {
	tests := []struct {
		cmd       string
		dangerous bool
	}{
		{"rm -rf /", true},
		{"rm -rf /usr", true},
		{"rm -rf /etc", true},
		{"rm /var", true},
		{"rmdir /bin", true},
		{"rm -rf ./tmp", false},
		{"rm -rf /tmp/test", false},
		{"rm file.txt", false},
		{"ls -la", false},
	}

	for _, tt := range tests {
		result := isDangerousRemovalTarget(tt.cmd)
		if result != tt.dangerous {
			t.Errorf("isDangerousRemovalTarget(%q) = %v, want %v", tt.cmd, result, tt.dangerous)
		}
	}
}
