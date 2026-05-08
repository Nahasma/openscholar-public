package kb

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
)

// PageIndexHealth captures the health status of the PageIndex environment.
type PageIndexHealth struct {
	Available       bool      `json:"available"`
	PythonPath      string    `json:"python_path,omitempty"`
	SSLVersion      string    `json:"ssl_version,omitempty"`
	SSLOk           bool      `json:"ssl_ok"`
	OpenAIPackageOk bool      `json:"openai_package_ok"`
	OpenAIKeyOk     bool      `json:"key_available"`
	CheckedAt       time.Time `json:"checked_at"`
}

var (
	cachedHealth    *PageIndexHealth
	cachedHealthKey string // pythonPath used for the cached result
	cachedHealthMu  sync.Mutex
	healthCacheTTL  = 5 * time.Minute
)

// CheckPageIndexHealth runs lightweight health checks on the Python environment.
// Results are cached for 5 minutes, keyed by pythonPath.
func CheckPageIndexHealth(pythonPath string) PageIndexHealth {
	cachedHealthMu.Lock()
	defer cachedHealthMu.Unlock()

	if cachedHealth != nil && cachedHealthKey == pythonPath && time.Since(cachedHealth.CheckedAt) < healthCacheTTL {
		return *cachedHealth
	}

	h := PageIndexHealth{
		CheckedAt:  time.Now(),
		PythonPath: pythonPath,
	}

	// Check 1: Python SSL version
	var sslOut bytes.Buffer
	sslCmd := exec.Command(pythonPath, "-c", "import ssl; print(ssl.OPENSSL_VERSION)")
	sslCmd.Stdout = &sslOut
	sslCmd.Stderr = &bytes.Buffer{} // suppress stderr
	if err := sslCmd.Run(); err == nil {
		h.SSLVersion = strings.TrimSpace(sslOut.String())
		h.SSLOk = isSSLVersionOk(h.SSLVersion)
	}

	// Check 2: OpenAI package available
	checkCmd := exec.Command(pythonPath, "-c", "import openai")
	checkCmd.Stderr = &bytes.Buffer{}
	h.Available = checkCmd.Run() == nil
	h.OpenAIPackageOk = h.Available

	// Check 3: API key configured
	if cfg := config.Get(); cfg != nil {
		for _, prov := range []models.ModelProvider{models.ProviderOpenAI, models.ProviderOpenAICompatible, models.ProviderDeepSeek, models.ProviderSiliconFlow} {
			if p, ok := cfg.Providers[prov]; ok && p.APIKey != "" {
				h.OpenAIKeyOk = true
				break
			}
		}
	}

	cachedHealth = &h
	cachedHealthKey = pythonPath
	return h
}

// isSSLVersionOk checks if the SSL implementation is OpenSSL >= 1.1.1.
// Expected format: "OpenSSL 1.1.1k  25 Mar 2021" or "OpenSSL 3.0.2 ...".
// LibreSSL is intentionally not treated as OpenSSL-compatible by version number.
func isSSLVersionOk(version string) bool {
	parts := strings.Fields(version)
	if len(parts) < 2 {
		return false
	}
	if parts[0] != "OpenSSL" {
		return false
	}
	ver := parts[1] // e.g., "1.1.1k" or "3.0.2"

	// Split by "." and compare major.minor.patch
	segments := strings.SplitN(ver, ".", 3)
	if len(segments) < 3 {
		return false
	}

	major, err := strconv.Atoi(segments[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(segments[1])
	if err != nil {
		return false
	}
	patch := 0
	for _, r := range segments[2] {
		if r < '0' || r > '9' {
			break
		}
		patch = patch*10 + int(r-'0')
	}

	if major > 1 {
		return true
	}
	if major == 1 && minor > 1 {
		return true
	}
	return major == 1 && minor == 1 && patch >= 1
}

// ResetHealthCache clears the cached health check (used in tests).
func ResetHealthCache() {
	cachedHealthMu.Lock()
	defer cachedHealthMu.Unlock()
	cachedHealth = nil
}
