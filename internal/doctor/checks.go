package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
)

// Status represents the result of a dependency check.
type Status int

const (
	StatusOK   Status = iota // Dependency is available and working
	StatusWarn               // Optional dependency missing or degraded
	StatusFail               // Required dependency missing
)

// Check represents a single dependency check result.
type Check struct {
	Name   string // Display name (e.g., "python3", "PageIndex")
	Status Status
	Detail string // Current state (e.g., "/usr/bin/python3 (3.12.0)")
	Fix    string // How to install (platform-aware)
}

// RunChecks performs all dependency checks and returns the results.
func RunChecks() []Check {
	var checks []Check

	checks = append(checks, checkPython()...)
	checks = append(checks, checkConfig())
	checks = append(checks, checkAPIKey())

	return checks
}

// checkPython checks python3 and Python packages (PyMuPDF, PageIndex, MinerU).
func checkPython() []Check {
	var checks []Check

	// python3 (check venv first, then system)
	pythonPath := config.PythonPath()
	if pythonPath == "" {
		checks = append(checks, Check{
			Name:   "python3",
			Status: StatusFail,
			Detail: "NOT FOUND",
			Fix:    pythonInstallHint(),
		})
		// If python3 is missing, skip all Python package checks
		checks = append(checks,
			Check{Name: "PyMuPDF (fitz)", Status: StatusFail, Detail: "requires python3"},
			Check{Name: "PageIndex", Status: StatusFail, Detail: "requires python3"},
			Check{Name: "MinerU", Status: StatusWarn, Detail: "requires python3 (optional)"},
		)
		return checks
	}

	// Get python version
	version := getPythonVersion(pythonPath)
	checks = append(checks, Check{
		Name:   "python3",
		Status: StatusOK,
		Detail: fmt.Sprintf("%s (%s)", pythonPath, version),
	})

	// PyMuPDF (fitz)
	if checkPythonImport(pythonPath, "fitz") {
		checks = append(checks, Check{
			Name:   "PyMuPDF (fitz)",
			Status: StatusOK,
			Detail: "installed",
		})
	} else {
		checks = append(checks, Check{
			Name:   "PyMuPDF (fitz)",
			Status: StatusFail,
			Detail: "NOT FOUND",
			Fix:    "pip install PyMuPDF",
		})
	}

	// PageIndex
	if checkPythonImport(pythonPath, "pageindex") {
		checks = append(checks, Check{
			Name:   "PageIndex",
			Status: StatusOK,
			Detail: "installed",
		})
	} else {
		checks = append(checks, Check{
			Name:   "PageIndex",
			Status: StatusWarn,
			Detail: "not installed (optional; KB uses basic mode)",
		})
	}

	// MinerU (optional)
	if _, err := exec.LookPath("mineru"); err == nil {
		checks = append(checks, Check{
			Name:   "MinerU",
			Status: StatusOK,
			Detail: "installed",
		})
	} else {
		checks = append(checks, Check{
			Name:   "MinerU",
			Status: StatusWarn,
			Detail: "not installed (optional, for high-accuracy PDF)",
			Fix:    "pip install magic-pdf",
		})
	}

	return checks
}

// checkConfig checks if the configuration file exists.
func checkConfig() Check {
	configPath := config.DataPath("config.json")
	if _, err := os.Stat(configPath); err == nil {
		return Check{
			Name:   "Config",
			Status: StatusOK,
			Detail: ".openscholar/config.json",
		}
	}
	return Check{
		Name:   "Config",
		Status: StatusFail,
		Detail: "NOT FOUND",
		Fix:    "openscholar init",
	}
}

// checkAPIKey checks if at least one API key is configured.
func checkAPIKey() Check {
	cfg := config.Get()
	if cfg == nil {
		return Check{
			Name:   "API Key",
			Status: StatusFail,
			Detail: "config not loaded",
			Fix:    "openscholar init",
		}
	}

	for provider, provCfg := range cfg.Providers {
		if provCfg.APIKey != "" {
			masked := provCfg.APIKey
			if len(masked) > 8 {
				masked = masked[:4] + "..." + masked[len(masked)-4:]
			}
			return Check{
				Name:   "API Key",
				Status: StatusOK,
				Detail: fmt.Sprintf("%s (%s)", provider, masked),
			}
		}
	}

	return Check{
		Name:   "API Key",
		Status: StatusFail,
		Detail: "no API key configured",
		Fix:    "openscholar init",
	}
}

// checkPythonImport tries to import a Python module and returns success.
func checkPythonImport(pythonPath, module string) bool {
	cmd := exec.Command(pythonPath, "-c", fmt.Sprintf("import %s", module))
	return cmd.Run() == nil
}

// getPythonVersion returns the Python version string.
func getPythonVersion(pythonPath string) string {
	out, err := exec.Command(pythonPath, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "Python "))
}

// pythonInstallHint returns a platform-aware hint for installing python3.
func pythonInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install python3"
	case "linux":
		return "sudo apt install python3  # or: sudo dnf install python3"
	default:
		return "Install Python 3 from https://python.org"
	}
}
