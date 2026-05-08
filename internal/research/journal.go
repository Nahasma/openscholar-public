package research

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/research/orchestrator"
)

const journalFileName = "experiment-journal.jsonl"

// JournalEntry 实验日志条目
type JournalEntry struct {
	ID           string             `json:"id"`
	Timestamp    int64              `json:"timestamp"`
	NodeID       string             `json:"node_id,omitempty"`
	Stage        string             `json:"stage"`
	Hypothesis   string             `json:"hypothesis"`
	CodePath     string             `json:"code_path,omitempty"`
	Metrics      map[string]float64 `json:"metrics,omitempty"`
	Observations string             `json:"observations"`
	Figures      []string           `json:"figures,omitempty"`
	Status       string             `json:"status"`
	DurationSec  int                `json:"duration_sec,omitempty"`
}

// AppendEntry 追加一条实验日志
func AppendEntry(workDir string, entry *JournalEntry) error {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}
	if entry.ID == "" {
		count, _ := countEntries(workDir)
		entry.ID = fmt.Sprintf("exp-%03d", count+1)
	}

	path := filepath.Join(workDir, journalFileName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// AppendDebugEntry 记录 debug 过程
func AppendDebugEntry(workDir string, nodeID string, attempt int, errorLog string, fix string) error {
	return AppendEntry(workDir, &JournalEntry{
		NodeID:       nodeID,
		Stage:        "debug",
		Hypothesis:   fmt.Sprintf("修复尝试 #%d", attempt),
		Observations: fmt.Sprintf("错误: %s\n修复: %s", truncateStr(errorLog, 500), fix),
		Status:       "buggy",
	})
}

// ReadJournal 读取完整实验日志
func ReadJournal(workDir string) ([]*JournalEntry, error) {
	path := filepath.Join(workDir, journalFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []*JournalEntry
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

// SummarizeJournal 生成日志摘要文本（供 Writer Agent 消费）
func SummarizeJournal(workDir string) (string, error) {
	entries, err := ReadJournal(workDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "（无实验日志）", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 条实验记录：\n\n", len(entries)))

	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("### %s [%s] — %s\n", e.ID, e.Stage, e.Status))
		sb.WriteString(fmt.Sprintf("- 假设: %s\n", e.Hypothesis))
		if len(e.Metrics) > 0 {
			sb.WriteString("- 指标: ")
			for k, v := range e.Metrics {
				sb.WriteString(fmt.Sprintf("%s=%.4f ", k, v))
			}
			sb.WriteString("\n")
		}
		if e.Observations != "" {
			sb.WriteString(fmt.Sprintf("- 观察: %s\n", e.Observations))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// AppendFromAssessment 从 assessment 和 contract 写 journal 条目
func AppendFromAssessment(workDir string, assessment *orchestrator.Assessment, contract *orchestrator.TaskContract) error {
	// 从 manifest 获取 metrics
	var metrics map[string]float64
	var durationSec int
	if m, err := orchestrator.LoadManifest(workDir, assessment.NodeID); err == nil {
		metrics = m.Metrics
		durationSec = int(m.DurationSec)
	}

	return AppendEntry(workDir, &JournalEntry{
		NodeID:      assessment.NodeID,
		Stage:       contract.NodeType,
		Hypothesis:  contract.Background.Hypothesis,
		Metrics:     metrics,
		Status:      assessment.Status,
		DurationSec: durationSec,
		Observations: fmt.Sprintf("Score: %.0f%%, Checks: %d, Issues: %d",
			assessment.Score*100, len(assessment.Checks), len(assessment.Issues)),
	})
}

func countEntries(workDir string) (int, error) {
	entries, err := ReadJournal(workDir)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
