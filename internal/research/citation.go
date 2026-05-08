package research

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/openscholar/openscholar/internal/fileop"
)

// Evidence 表示一条引用证据（claim 到论文段落的映射）
type Evidence struct {
	Claim    string `json:"claim"`
	PaperID  string `json:"paper_id"`
	Page     int    `json:"page"`
	Quote    string `json:"quote"`
	Verified bool   `json:"verified"`
}

// LoadEvidence 从工作目录加载引用证据
func LoadEvidence(workDir string) ([]Evidence, error) {
	path := filepath.Join(workDir, ".citations", "evidence.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var evidence []Evidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}

// SaveEvidence 追加写入引用证据（不覆盖已有）
func SaveEvidence(workDir string, newEvidence []Evidence) error {
	existing, _ := LoadEvidence(workDir)
	all := append(existing, newEvidence...)
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(workDir, ".citations", "evidence.json")
	return fileop.SafeWrite(path, data, fileop.WithMkdir())
}

// CoverageRate 计算引用覆盖率（verified / total）
func CoverageRate(evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0
	}
	verified := 0
	for _, e := range evidence {
		if e.Verified {
			verified++
		}
	}
	return float64(verified) / float64(len(evidence))
}
