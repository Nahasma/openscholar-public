package config

import (
	"path/filepath"
	"testing"
)

func TestDataDirectory_Default(t *testing.T) {
	// cfg == nil → fallback to WorkingDirectory() + defaultDataDirectory
	old := cfg
	cfg = nil
	defer func() { cfg = old }()

	result := DataDirectory()
	wd := WorkingDirectory()
	expected := filepath.Join(wd, defaultDataDirectory)
	if result != expected {
		t.Errorf("DataDirectory() = %q, want %q", result, expected)
	}
}

func TestDataDirectory_RelativePath(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/home/user/project",
		Data:       DataConfig{Directory: "custom-data"},
	}
	defer func() { cfg = old }()

	result := DataDirectory()
	expected := "/home/user/project/custom-data"
	if result != expected {
		t.Errorf("DataDirectory() = %q, want %q", result, expected)
	}
}

func TestDataDirectory_AbsolutePath(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/home/user/project",
		Data:       DataConfig{Directory: "/abs/data"},
	}
	defer func() { cfg = old }()

	result := DataDirectory()
	expected := "/abs/data"
	if result != expected {
		t.Errorf("DataDirectory() = %q, want %q", result, expected)
	}
}

func TestDataPath(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/home/user/project",
		Data:       DataConfig{Directory: "my-data"},
	}
	defer func() { cfg = old }()

	result := DataPath("papers")
	expected := "/home/user/project/my-data/papers"
	if result != expected {
		t.Errorf("DataPath(\"papers\") = %q, want %q", result, expected)
	}

	result2 := DataPath("sub", "dir")
	expected2 := "/home/user/project/my-data/sub/dir"
	if result2 != expected2 {
		t.Errorf("DataPath(\"sub\", \"dir\") = %q, want %q", result2, expected2)
	}
}

func TestLayered_DefaultPaths(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/workspace/project",
		Data:       DataConfig{Directory: ".openscholar"},
		Paths:      PathsConfig{Root: ".openscholar"},
	}
	defer func() { cfg = old }()

	if got, want := PathsStateDir(), "/workspace/project/.openscholar/state"; got != want {
		t.Fatalf("PathsStateDir() = %q, want %q", got, want)
	}
	if got, want := PathsLogDir(), "/workspace/project/.openscholar/logs"; got != want {
		t.Fatalf("PathsLogDir() = %q, want %q", got, want)
	}
	if got, want := DBPath(), "/workspace/project/.openscholar/state/openscholar.db"; got != want {
		t.Fatalf("DBPath() = %q, want %q", got, want)
	}
	if got, want := StatePath("plans"), "/workspace/project/.openscholar/state/plans"; got != want {
		t.Fatalf("StatePath(\"plans\") = %q, want %q", got, want)
	}
	if got, want := RuntimePath("cron.pid"), "/workspace/project/.openscholar/runtime/cron.pid"; got != want {
		t.Fatalf("RuntimePath(\"cron.pid\") = %q, want %q", got, want)
	}
}

func TestLayered_AbsoluteAndRelativeOverrides(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/workspace/project",
		Data:       DataConfig{Directory: ".openscholar"},
		Paths: PathsConfig{
			Root:    "ignored-root",
			State:   "/var/lib/openscholar/state",
			Runtime: "runtime-data",
			Logs:    "log-data",
			Cache:   "cache-data",
		},
	}
	defer func() { cfg = old }()

	if got, want := StatePath("scheduled_tasks.json"), "/var/lib/openscholar/state/scheduled_tasks.json"; got != want {
		t.Fatalf("StatePath(...) = %q, want %q", got, want)
	}
	if got, want := RuntimePath("cron.pid"), "/workspace/project/runtime-data/cron.pid"; got != want {
		t.Fatalf("RuntimePath(...) = %q, want %q", got, want)
	}
	if got, want := LogDir(), "/workspace/project/log-data"; got != want {
		t.Fatalf("LogDir() = %q, want %q", got, want)
	}
	if got, want := VenvDir(), "/workspace/project/cache-data/python/venv"; got != want {
		t.Fatalf("VenvDir() = %q, want %q", got, want)
	}
	if got, want := DBPath(), "/var/lib/openscholar/state/openscholar.db"; got != want {
		t.Fatalf("DBPath() = %q, want %q", got, want)
	}
}

func TestLegacyHelpersRemainFlat(t *testing.T) {
	old := cfg
	cfg = &Config{
		WorkingDir: "/workspace/project",
		Data:       DataConfig{Directory: ".openscholar"},
	}
	defer func() { cfg = old }()

	if got, want := DBPath(), "/workspace/project/.openscholar/openscholar.db"; got != want {
		t.Fatalf("DBPath() = %q, want %q", got, want)
	}
	if got, want := StatePath("plans"), "/workspace/project/.openscholar/plans"; got != want {
		t.Fatalf("StatePath(\"plans\") = %q, want %q", got, want)
	}
	if got, want := RuntimePath("cron.pid"), "/workspace/project/.openscholar/cron.pid"; got != want {
		t.Fatalf("RuntimePath(\"cron.pid\") = %q, want %q", got, want)
	}
	if got, want := VenvDir(), "/workspace/project/.openscholar/venv"; got != want {
		t.Fatalf("VenvDir() = %q, want %q", got, want)
	}
}

func TestDefaultDataDir(t *testing.T) {
	result := DefaultDataDir()
	if result != defaultDataDirectory {
		t.Errorf("DefaultDataDir() = %q, want %q", result, defaultDataDirectory)
	}
}
