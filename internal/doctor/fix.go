package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
)

// FixResult describes the outcome of an auto-fix attempt.
type FixResult struct {
	Action  string // What was done
	Success bool
	Error   string // If failed
}

// AutoFix creates a venv (if needed) and installs missing Python dependencies.
// Returns a list of actions taken.
func AutoFix(checks []Check) []FixResult {
	var results []FixResult

	// Check if python3 is available at all
	systemPython, err := exec.LookPath("python3")
	if err != nil {
		results = append(results, FixResult{
			Action:  "Find python3",
			Success: false,
			Error:   "python3 not found in PATH. Please install Python 3 first.",
		})
		return results
	}

	// Ensure venv exists
	venvDir := config.VenvDir()
	venvPython := config.VenvPython()

	if _, err := os.Stat(venvPython); err != nil {
		results = append(results, createVenv(systemPython, venvDir))
		if !results[len(results)-1].Success {
			return results
		}
	} else {
		results = append(results, FixResult{
			Action:  fmt.Sprintf("venv at %s", shortenPath(venvDir)),
			Success: true,
		})
	}

	// Determine which packages need installing
	pip := config.VenvPip()
	python := config.VenvPython()

	// PyMuPDF
	if !checkPythonImport(python, "fitz") {
		results = append(results, pipInstall(pip, "PyMuPDF"))
	}

	// PageIndex is optional. If a local checkout is present, link it into the
	// venv; otherwise keep the basic KB indexing fallback.
	if !checkPythonImport(python, "pageindex") {
		pageindexPath := findPageIndexPath()
		if pageindexPath != "" {
			reqFile := filepath.Join(pageindexPath, "requirements.txt")
			if _, err := os.Stat(reqFile); err == nil {
				results = append(results, pipInstallRequirements(pip, reqFile))
			}
			// Create a .pth file in venv site-packages so "import pageindex" works
			results = append(results, installPthFile(python, pageindexPath))
		}
	}

	return results
}

// createVenv creates a Python venv at the given path.
func createVenv(pythonPath, venvDir string) FixResult {
	action := fmt.Sprintf("Create venv at %s", shortenPath(venvDir))

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(venvDir), 0o755); err != nil {
		return FixResult{Action: action, Success: false, Error: err.Error()}
	}

	cmd := exec.Command(pythonPath, "-m", "venv", venvDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return FixResult{Action: action, Success: false, Error: fmt.Sprintf("%s: %s", err, string(output))}
	}

	return FixResult{Action: action, Success: true}
}

// pipInstall installs a package via pip.
func pipInstall(pip, pkg string) FixResult {
	action := fmt.Sprintf("pip install %s", pkg)
	cmd := exec.Command(pip, "install", pkg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return FixResult{Action: action, Success: false, Error: trimOutput(string(output))}
	}
	return FixResult{Action: action, Success: true}
}

// pipInstallRequirements installs packages from a requirements.txt file.
func pipInstallRequirements(pip, reqFile string) FixResult {
	action := fmt.Sprintf("pip install -r %s", shortenPath(reqFile))
	cmd := exec.Command(pip, "install", "-r", reqFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return FixResult{Action: action, Success: false, Error: trimOutput(string(output))}
	}
	return FixResult{Action: action, Success: true}
}

// installPthFile creates a .pth file in the venv's site-packages so that
// "import pageindex" resolves to ref/PageIndex/pageindex/.
func installPthFile(python, packageDir string) FixResult {
	action := "Link PageIndex to venv"

	// Get the site-packages path from the venv python
	cmd := exec.Command(python, "-c", "import site; print(site.getsitepackages()[0])")
	out, err := cmd.Output()
	if err != nil {
		return FixResult{Action: action, Success: false, Error: "failed to find site-packages"}
	}

	sitePackages := strings.TrimSpace(string(out))
	pthFile := filepath.Join(sitePackages, "pageindex.pth")

	// Write the parent directory of pageindex/ so "import pageindex" works
	if err := os.WriteFile(pthFile, []byte(packageDir+"\n"), 0o644); err != nil {
		return FixResult{Action: action, Success: false, Error: err.Error()}
	}

	return FixResult{Action: action, Success: true}
}

// findPageIndexPath locates an optional local ref/PageIndex checkout.
func findPageIndexPath() string {
	cwd := config.WorkingDirectory()

	// Check relative to working directory
	candidate := filepath.Join(cwd, "ref", "PageIndex")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Check relative to executable
	if exePath, err := os.Executable(); err == nil {
		candidate = filepath.Join(filepath.Dir(exePath), "ref", "PageIndex")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// shortenPath replaces the working directory prefix with a relative path.
func shortenPath(p string) string {
	cwd := config.WorkingDirectory()
	if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

// trimOutput returns the last meaningful line of command output.
func trimOutput(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return s
	}
	// Return last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return s
}

// FormatFixResults formats fix results for display.
func FormatFixResults(results []FixResult) string {
	var sb strings.Builder
	allOK := true

	for _, r := range results {
		if r.Success {
			fmt.Fprintf(&sb, "  [OK] %s\n", r.Action)
		} else {
			fmt.Fprintf(&sb, "  [!!] %s: %s\n", r.Action, r.Error)
			allOK = false
		}
	}

	sb.WriteString("\n")
	if allOK {
		sb.WriteString("All dependencies installed successfully.\n")
	} else {
		sb.WriteString("Some installations failed. See errors above.\n")
	}

	return sb.String()
}
