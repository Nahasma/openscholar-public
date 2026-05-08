package config

import (
	"os"
	"path/filepath"
)

var cfg *Config

// Get returns the loaded config singleton.
func Get() *Config {
	return cfg
}

// Reset clears the cached config singleton, forcing the next Load() to re-read.
func Reset() {
	cfg = nil
}

// DefaultImageProvider returns the configured image generation provider name, or empty string.
func DefaultImageProvider() string {
	if cfg == nil {
		return ""
	}
	return cfg.ImageProvider
}

// WorkingDirectory returns the current working directory from config, or os.Getwd() if not loaded.
func WorkingDirectory() string {
	if cfg == nil {
		wd, _ := os.Getwd()
		return wd
	}
	return cfg.WorkingDir
}

// SetWorkingDirectory updates the workspace directory at runtime (e.g., from TUI dialog).
func SetWorkingDirectory(dir string) {
	if cfg != nil {
		cfg.WorkingDir = dir
	}
}

// DataDirectory 返回项目数据目录的绝对路径。
// 尊重 config.Data.Directory；相对路径基于 WorkingDirectory() 解析。
func DataDirectory() string {
	dir := defaultDataDirectory
	if cfg != nil && cfg.Data.Directory != "" {
		dir = cfg.Data.Directory
	}
	return resolvePath(dir)
}

// DataPath 返回数据目录下子路径的绝对路径。
// 等价于 filepath.Join(DataDirectory(), elem...)。
func DataPath(elem ...string) string {
	return filepath.Join(append([]string{DataDirectory()}, elem...)...)
}

// DefaultDataDir 返回默认数据目录名（供 init 层在 config 未加载时使用）。
func DefaultDataDir() string {
	return defaultDataDirectory
}

// LogDir returns the directory for debug session logs.
func LogDir() string {
	if cfg == nil {
		return ""
	}
	if hasPathsConfig() {
		return PathsLogDir()
	}
	return filepath.Join(DataDirectory(), "logs")
}

func hasPathsConfig() bool {
	if cfg == nil {
		return false
	}
	p := cfg.Paths
	return p.Root != "" || p.Config != "" || p.State != "" || p.Extensions != "" || p.Cache != "" || p.Logs != "" || p.Runtime != ""
}

func resolvePath(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(WorkingDirectory(), path)
}

func pathsRoot() string {
	if cfg != nil && cfg.Paths.Root != "" {
		return cfg.Paths.Root
	}
	return defaultDataDirectory
}

// PathsRootDir returns the absolute control root for layered storage.
func PathsRootDir() string { return resolvePath(pathsRoot()) }

// PathsConfigDir returns the absolute config directory for layered storage.
func PathsConfigDir() string {
	if cfg != nil && cfg.Paths.Config != "" {
		return resolvePath(cfg.Paths.Config)
	}
	return filepath.Join(PathsRootDir(), "config")
}

// PathsStateDir returns the absolute state directory for layered storage.
func PathsStateDir() string {
	if cfg != nil && cfg.Paths.State != "" {
		return resolvePath(cfg.Paths.State)
	}
	return filepath.Join(PathsRootDir(), "state")
}

// PathsExtensionsDir returns the absolute extensions directory for layered storage.
func PathsExtensionsDir() string {
	if cfg != nil && cfg.Paths.Extensions != "" {
		return resolvePath(cfg.Paths.Extensions)
	}
	return filepath.Join(PathsRootDir(), "extensions")
}

// PathsCacheDir returns the absolute cache directory for layered storage.
func PathsCacheDir() string {
	if cfg != nil && cfg.Paths.Cache != "" {
		return resolvePath(cfg.Paths.Cache)
	}
	return filepath.Join(PathsRootDir(), "cache")
}

// PathsLogDir returns the absolute logs directory for layered storage.
func PathsLogDir() string {
	if cfg != nil && cfg.Paths.Logs != "" {
		return resolvePath(cfg.Paths.Logs)
	}
	return filepath.Join(PathsRootDir(), "logs")
}

// PathsRuntimeDir returns the absolute runtime directory for layered storage.
func PathsRuntimeDir() string {
	if cfg != nil && cfg.Paths.Runtime != "" {
		return resolvePath(cfg.Paths.Runtime)
	}
	return filepath.Join(PathsRootDir(), "runtime")
}

// StatePath returns an absolute path under state storage.
func StatePath(elem ...string) string {
	base := DataDirectory()
	if hasPathsConfig() {
		base = PathsStateDir()
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// RuntimePath returns an absolute path under runtime storage.
func RuntimePath(elem ...string) string {
	base := DataDirectory()
	if hasPathsConfig() {
		base = PathsRuntimeDir()
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// CachePath returns an absolute path under cache storage.
func CachePath(elem ...string) string {
	base := DataDirectory()
	if hasPathsConfig() {
		base = PathsCacheDir()
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// ExtensionsPath returns an absolute path under extensions storage.
func ExtensionsPath(elem ...string) string {
	base := DataDirectory()
	if hasPathsConfig() {
		base = PathsExtensionsDir()
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// DBPath returns the SQLite database file path.
func DBPath() string {
	if hasPathsConfig() {
		return filepath.Join(PathsStateDir(), "openscholar.db")
	}
	return filepath.Join(DataDirectory(), "openscholar.db")
}

// SessionLogEnabled returns true if full session logging is enabled.
// Activates when either sessionLog or debug is set in config.
func SessionLogEnabled() bool {
	return cfg != nil && (cfg.SessionLog || cfg.Debug)
}
