package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openscholar/openscholar/internal/fileop"
)

// Manifest records the full execution metadata for a node run.
type Manifest struct {
	NodeID         string             `json:"node_id"`
	PipelineID     string             `json:"pipeline_id"`
	ContractHash   string             `json:"contract_hash"`
	Provider       string             `json:"provider"`
	SessionID      string             `json:"session_id"`
	Status         string             `json:"status"`
	StartedAt      time.Time          `json:"started_at"`
	EndedAt        time.Time          `json:"ended_at"`
	DurationSec    float64            `json:"duration_sec"`
	ExitReason     string             `json:"exit_reason"`
	Metrics        map[string]float64 `json:"metrics"`
	Artifacts      []ArtifactEntry    `json:"artifacts"`
	Missing        []string           `json:"missing"`
	Warnings       []string           `json:"warnings"`
	ValidationPass bool               `json:"validation_pass"`
}

// nodeDir returns the canonical directory for a node inside workDir.
func nodeDir(workDir, nodeID string) string {
	return filepath.Join(workDir, "experiments", "nodes", nodeID)
}

// Save writes the manifest as manifest.json into the node's directory.
// The directory is created if it does not exist.
func (m *Manifest) Save(workDir string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest: marshal: %w", err)
	}
	dest := filepath.Join(nodeDir(workDir, m.NodeID), "manifest.json")
	return fileop.SafeWrite(dest, data, fileop.WithMkdir())
}

// LoadManifest reads the manifest.json for a node from workDir.
func LoadManifest(workDir, nodeID string) (*Manifest, error) {
	path := filepath.Join(nodeDir(workDir, nodeID), "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: unmarshal: %w", err)
	}
	return &m, nil
}
