package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
)

type cdCmd struct{}

func (c *cdCmd) Name() string        { return "cd" }
func (c *cdCmd) Description() string { return "切换工作目录" }

func (c *cdCmd) Execute(ctx command.Context) command.Result {
	path := strings.TrimSpace(ctx.Args)
	if path == "" {
		cwd, _ := os.Getwd()
		return command.Result{Output: fmt.Sprintf("当前目录: %s", cwd)}
	}

	// Support ~ home directory expansion
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return command.Result{Error: fmt.Errorf("无法获取用户目录: %w", err)}
		}
		path = filepath.Join(home, path[1:])
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return command.Result{Error: fmt.Errorf("无效路径: %w", err)}
	}

	// Check directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return command.Result{Error: fmt.Errorf("目录不存在: %s", absPath)}
	}
	if !info.IsDir() {
		return command.Result{Error: fmt.Errorf("不是目录: %s", absPath)}
	}

	if err := os.Chdir(absPath); err != nil {
		return command.Result{Error: fmt.Errorf("切换目录失败: %w", err)}
	}

	return command.Result{
		Output: fmt.Sprintf("已切换到: %s", absPath),
		Action: "cd",
	}
}
