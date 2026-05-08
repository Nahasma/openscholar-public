package fileop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileOperation string

const (
	FileOpRead   FileOperation = "read"
	FileOpSearch FileOperation = "search"
	FileOpEdit   FileOperation = "edit"
	FileOpWrite  FileOperation = "write"
)

type ResolvedPath struct {
	Raw      string
	Abs      string
	Resolved string
	Exists   bool
	IsDir    bool
}

func ResolvePath(rawPath, baseDir string) (ResolvedPath, error) {
	if strings.TrimSpace(rawPath) == "" {
		return ResolvedPath{}, fmt.Errorf("empty path")
	}
	p := rawPath
	if !filepath.IsAbs(p) {
		if baseDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return ResolvedPath{}, fmt.Errorf("resolve working directory: %w", err)
			}
			baseDir = wd
		}
		p = filepath.Join(baseDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve absolute path: %w", err)
	}
	resolved := abs
	st, statErr := os.Stat(abs)
	exists := statErr == nil
	isDir := exists && st.IsDir()
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = r
	} else if os.IsNotExist(err) {
		dir := filepath.Dir(abs)
		if rd, derr := filepath.EvalSymlinks(dir); derr == nil {
			resolved = filepath.Join(rd, filepath.Base(abs))
		}
	}
	return ResolvedPath{Raw: rawPath, Abs: filepath.Clean(abs), Resolved: filepath.Clean(resolved), Exists: exists, IsDir: isDir}, nil
}

func InBoundary(targetPath, boundary string) bool {
	if boundary == "" {
		return true
	}
	t := filepath.Clean(targetPath)
	b := filepath.Clean(boundary)
	return t == b || strings.HasPrefix(t, b+string(filepath.Separator))
}
