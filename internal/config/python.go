package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// VenvDir returns the path to the project-local Python venv.
func VenvDir() string {
	if hasPathsConfig() {
		return CachePath("python", "venv")
	}
	return DataPath("venv")
}

// VenvPython returns the python3 binary path inside the venv.
func VenvPython() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(VenvDir(), "Scripts", "python.exe")
	}
	return filepath.Join(VenvDir(), "bin", "python3")
}

// VenvPip returns the pip binary path inside the venv.
func VenvPip() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(VenvDir(), "Scripts", "pip.exe")
	}
	return filepath.Join(VenvDir(), "bin", "pip")
}

// PythonPath returns the best available python3 path.
// Priority: venv python → system python → empty string.
func PythonPath() string {
	// Check venv first
	venvPy := VenvPython()
	if _, err := exec.LookPath(venvPy); err == nil {
		return venvPy
	}

	// Fall back to system python
	if sysPath, err := exec.LookPath("python3"); err == nil {
		return sysPath
	}

	return ""
}
