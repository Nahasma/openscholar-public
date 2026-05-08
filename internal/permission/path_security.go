package permission

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// validatePathSecurity 检查路径中的 Shell 展开注入向量。
// 在 Request() Step 1 之前调用，对 fileTools 生效。
func validatePathSecurity(path string, writeOp bool) error {
	// 1. 拒绝 UNC 网络路径
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `//`) {
		return fmt.Errorf("UNC network path not allowed: %s", path)
	}
	// 2. 拒绝含 Shell 展开字符的路径
	if strings.ContainsAny(path, "$%") || strings.Contains(path, "$(") || strings.Contains(path, "`") {
		return fmt.Errorf("path contains shell expansion syntax, rejected: %s", path)
	}
	// 3. 拒绝 ~user 变体（允许 ~ 和 ~/）
	if strings.HasPrefix(path, "~") && path != "~" && !strings.HasPrefix(path, "~/") {
		return fmt.Errorf("tilde path variant not allowed: %s", path)
	}
	// 4. 写操作拒绝 glob 模式
	if writeOp && strings.ContainsAny(path, "*?[{") {
		return fmt.Errorf("write operation path must not contain glob patterns: %s", path)
	}
	return nil
}

// isWriteToolOp 判断工具是否为写操作
func isWriteToolOp(toolName string) bool {
	switch toolName {
	case "Edit", "Write":
		return true
	}
	return false
}

// resolveAndValidatePath 解析符号链接并验证最终目标仍在工作区内。
func resolveAndValidatePath(rawPath, workspaceDir string) (string, error) {
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}

	// 尝试解析符号链接
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = resolved
	} else {
		// 路径不存在时，解析父目录的符号链接（允许新建文件）
		dir := filepath.Dir(absPath)
		if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil {
			absPath = filepath.Join(resolvedDir, filepath.Base(absPath))
		}
	}

	// 检查解析后的目标是否仍在工作区内
	if workspaceDir != "" {
		absWork, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("cannot resolve workspace: %w", err)
		}
		// 同样解析工作区的符号链接，保持一致性（macOS /var → /private/var 等）
		if resolvedWork, err := filepath.EvalSymlinks(absWork); err == nil {
			absWork = resolvedWork
		}
		absWork = filepath.Clean(absWork)
		if absPath != absWork && !strings.HasPrefix(absPath, absWork+string(filepath.Separator)) {
			return "", fmt.Errorf("resolved path escapes workspace: %s → %s", rawPath, absPath)
		}
	}
	return absPath, nil
}

// isDangerousRemovalTarget 检测 rm/rmdir 命令是否指向危险路径。
func isDangerousRemovalTarget(command string) bool {
	fields := strings.Fields(command)
	if len(fields) < 2 {
		return false
	}
	if fields[0] != "rm" && fields[0] != "rmdir" {
		return false
	}

	home := os.Getenv("HOME")
	dangerousPaths := []string{"/", "/usr", "/etc", "/var", "/bin", "/sbin", "/opt"}
	if home != "" {
		dangerousPaths = append(dangerousPaths, home)
	}

	for _, arg := range fields[1:] {
		if strings.HasPrefix(arg, "-") {
			continue // 跳过选项
		}
		cleaned := filepath.Clean(arg)
		if slices.Contains(dangerousPaths, cleaned) {
			return true
		}
	}
	return false
}
