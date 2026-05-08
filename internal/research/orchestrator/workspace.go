package orchestrator

import (
	"os"
	"path/filepath"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// InitNodeWorkspace creates the standard directory structure for an experiment node.
// Layout: experiments/nodes/<nodeID>/{input,work,outputs,outputs/figures,logs,snapshots}
func InitNodeWorkspace(workDir, nodeID string) error {
	base := filepath.Join(workDir, "experiments", "nodes", nodeID)
	for _, sub := range []string{
		"input",
		"work",
		"outputs",
		"outputs/figures",
		"logs",
		"snapshots",
	} {
		if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// InitExperimentDirs ensures the pipeline-level experiment directories exist.
// Call once after Engine.Create.
func InitExperimentDirs(workDir string) error {
	for _, dir := range []string{
		"experiments/nodes",
		"code/shared",
		"code/configs",
		"journal",
		"results/aggregate",
		"results/promoted",
	} {
		if err := os.MkdirAll(filepath.Join(workDir, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// CopySharedCode copies files from code/shared/ into the node's work/ directory.
func CopySharedCode(workDir, nodeID string) (copied []string, err error) {
	sharedDir := filepath.Join(workDir, "code", "shared")
	targetDir := filepath.Join(workDir, "experiments", "nodes", nodeID, "work")

	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(sharedDir, entry.Name())
		dst := filepath.Join(targetDir, entry.Name())
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			continue
		}
		if writeErr := fileop.WriteFileAtomic(dst, data, 0o644); writeErr != nil {
			continue
		}
		copied = append(copied, entry.Name())
	}
	return copied, nil
}
