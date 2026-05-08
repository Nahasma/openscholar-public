package orchestrator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ArtifactEntry describes a single output file found in the node's output directory.
type ArtifactEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// ArtifactReport summarises the artifact scan result for a node.
type ArtifactReport struct {
	NodeID   string             `json:"node_id"`
	Found    []ArtifactEntry    `json:"found"`
	Missing  []string           `json:"missing"`
	Extra    []ArtifactEntry    `json:"extra"`
	Metrics  map[string]float64 `json:"metrics"`
	Warnings []string           `json:"warnings"`
}

// CollectArtifacts scans the node outputs directory, checks expected files,
// parses the metric artifact if present, and falls back to [METRIC] markers in logs.
// metricArtifactPath is relative to the node directory (e.g. "outputs/metrics.json").
// If empty, defaults to "outputs/metrics.json".
func CollectArtifacts(workDir, nodeID string, expectedFiles []string, metricArtifactPath string) (*ArtifactReport, error) {
	if metricArtifactPath == "" {
		metricArtifactPath = "outputs/metrics.json"
	}
	nodeDir := filepath.Join(workDir, "experiments", "nodes", nodeID)
	outputsDir := filepath.Join(nodeDir, "outputs")

	report := &ArtifactReport{
		NodeID:  nodeID,
		Found:   []ArtifactEntry{},
		Missing: []string{},
		Extra:   []ArtifactEntry{},
		Metrics: map[string]float64{},
	}

	// Collect all files in the outputs directory.
	foundPaths := map[string]ArtifactEntry{}
	if info, err := os.Stat(outputsDir); err == nil && info.IsDir() {
		err = filepath.Walk(outputsDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(outputsDir, path)
			entry := ArtifactEntry{
				Path:     rel,
				Size:     fi.Size(),
				Checksum: sha256File(path),
			}
			foundPaths[rel] = entry
			report.Found = append(report.Found, entry)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Determine missing and extra files relative to expected list.
	expectedSet := map[string]bool{}
	for _, ef := range expectedFiles {
		expectedSet[ef] = true
		if _, ok := foundPaths[ef]; !ok {
			report.Missing = append(report.Missing, ef)
		}
	}
	for rel, entry := range foundPaths {
		if !expectedSet[rel] {
			report.Extra = append(report.Extra, entry)
		}
	}

	// Try to parse the metric artifact file.
	metricsFile := filepath.Join(nodeDir, metricArtifactPath)
	if metrics, err := parseMetricsJSON(metricsFile); err == nil && len(metrics) > 0 {
		report.Metrics = metrics
	} else {
		// Fall back to [METRIC] markers in stdout logs.
		logsDir := filepath.Join(nodeDir, "logs")
		if markers := parseMetricMarkers(logsDir); len(markers) > 0 {
			report.Metrics = markers
		}
	}

	return report, nil
}

// sha256File computes the SHA-256 hash of a file and returns the first 16 hex chars.
func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// parseMetricsJSON reads a JSON file containing a flat map[string]float64.
func parseMetricsJSON(path string) (map[string]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Minimal JSON parser: expect {"key": value, ...} with numeric values.
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	result := map[string]float64{}
	// Use encoding/json via a helper to avoid import cycle concerns.
	// We decode into map[string]interface{} and convert.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for k, v := range raw {
		switch val := v.(type) {
		case float64:
			result[k] = val
		case int:
			result[k] = float64(val)
		}
	}
	return result, nil
}

// parseMetricMarkers scans *.stdout.log files in logsDir for lines matching:
//
//	[METRIC] name=value
func parseMetricMarkers(logsDir string) map[string]float64 {
	result := map[string]float64{}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return result
	}

	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".stdout.log") {
			continue
		}
		path := filepath.Join(logsDir, de.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "[METRIC]") {
				continue
			}
			// Format: [METRIC] name=value
			rest := strings.TrimSpace(strings.TrimPrefix(line, "[METRIC]"))
			parts := strings.SplitN(rest, "=", 2)
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])
			if v, err := strconv.ParseFloat(valStr, 64); err == nil {
				result[name] = v
			}
		}
		f.Close()
	}

	return result
}

// ValidationIssue represents a single validation finding.
type ValidationIssue struct {
	Severity string `json:"severity"` // "error" or "warning"
	Check    string `json:"check"`
	Message  string `json:"message"`
}

// ValidateArtifacts inspects an ArtifactReport and returns any validation issues.
func ValidateArtifacts(report *ArtifactReport) []ValidationIssue {
	var issues []ValidationIssue

	// Missing deliverables → error.
	for _, m := range report.Missing {
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Check:    "missing_deliverable",
			Message:  "expected artifact not found: " + m,
		})
	}

	// Empty metrics → error.
	if len(report.Metrics) == 0 {
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Check:    "empty_metrics",
			Message:  "no metrics recorded for node " + report.NodeID,
		})
	}

	// NaN or Inf metric values → error.
	for name, val := range report.Metrics {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			issues = append(issues, ValidationIssue{
				Severity: "error",
				Check:    "invalid_metric",
				Message:  "metric " + name + " has invalid value (NaN or Inf)",
			})
		}
	}

	// Small figures (< 1 KB) → warning.
	for _, entry := range report.Found {
		ext := strings.ToLower(filepath.Ext(entry.Path))
		isFigure := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".svg" || ext == ".pdf"
		if isFigure && entry.Size < 1024 {
			issues = append(issues, ValidationIssue{
				Severity: "warning",
				Check:    "small_figure",
				Message:  "figure file " + entry.Path + " is suspiciously small (" + strconv.FormatInt(entry.Size, 10) + " bytes)",
			})
		}
	}

	return issues
}
