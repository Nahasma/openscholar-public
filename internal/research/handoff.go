package research

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/openscholar/openscholar/internal/fileop"
)

// HandoffTask 研究子任务（存储在 .handoff/tasks.jsonl）
type HandoffTask struct {
	ID          string `json:"id"`
	PhaseID     string `json:"phase_id"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending | claimed | completed | failed
	Owner       string `json:"owner"`
	Result      string `json:"result,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// HandoffMessage Agent 间通信消息（存储在 .handoff/messages.jsonl）
type HandoffMessage struct {
	ID        string `json:"id"`
	Sender    string `json:"sender"`
	Receiver  string `json:"receiver"` // 特定 agent 名或 "all" 表示广播
	MsgType   string `json:"msg_type"` // finding | request | broadcast
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// HandoffManager 管理 .handoff/ 目录下的任务和消息文件
type HandoffManager struct {
	workDir string // 研究工作区目录
}

// NewHandoffManager 创建一个新的 HandoffManager，workDir 为研究工作区根目录。
func NewHandoffManager(workDir string) *HandoffManager {
	return &HandoffManager{workDir: workDir}
}

func (m *HandoffManager) tasksPath() string {
	return filepath.Join(m.workDir, ".handoff", "tasks.jsonl")
}

func (m *HandoffManager) messagesPath() string {
	return filepath.Join(m.workDir, ".handoff", "messages.jsonl")
}

// ─── Task Management ────────────────────────────────────────────────────────

// CreateTask 创建一个新的 pending 子任务并追加到 tasks.jsonl。
func (m *HandoffManager) CreateTask(phaseID, subject, desc string) (*HandoffTask, error) {
	now := time.Now().Unix()
	task := &HandoffTask{
		ID:          uuid.NewString()[:8],
		PhaseID:     phaseID,
		Subject:     subject,
		Description: desc,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.appendTask(task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

// ClaimTask 将 pending 任务的 status 更新为 claimed，并设置 owner。
func (m *HandoffManager) ClaimTask(taskID, owner string) (*HandoffTask, error) {
	tasks, err := m.LoadTasks()
	if err != nil {
		return nil, err
	}

	var target *HandoffTask
	for _, t := range tasks {
		if t.ID == taskID {
			if t.Status != "pending" {
				return nil, fmt.Errorf("task %s is not pending (status: %s)", taskID, t.Status)
			}
			t.Status = "claimed"
			t.Owner = owner
			t.UpdatedAt = time.Now().Unix()
			target = t
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	if err := m.writeTasks(tasks); err != nil {
		return nil, err
	}
	return target, nil
}

// CompleteTask 将任务 status 更新为 completed，并记录结果。
func (m *HandoffManager) CompleteTask(taskID, result string) error {
	tasks, err := m.LoadTasks()
	if err != nil {
		return err
	}

	found := false
	for _, t := range tasks {
		if t.ID == taskID {
			t.Status = "completed"
			t.Result = result
			t.UpdatedAt = time.Now().Unix()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("task %s not found", taskID)
	}

	return m.writeTasks(tasks)
}

// LoadTasks 从 tasks.jsonl 加载所有任务。
func (m *HandoffManager) LoadTasks() ([]*HandoffTask, error) {
	return readJSONL[HandoffTask](m.tasksPath())
}

// ListUnclaimed 返回指定 phaseID 下所有 pending 状态的任务。
// phaseID 为空时返回所有 pending 任务。
func (m *HandoffManager) ListUnclaimed(phaseID string) ([]*HandoffTask, error) {
	tasks, err := m.LoadTasks()
	if err != nil {
		return nil, err
	}
	var result []*HandoffTask
	for _, t := range tasks {
		if t.Status == "pending" && (phaseID == "" || t.PhaseID == phaseID) {
			result = append(result, t)
		}
	}
	return result, nil
}

// ─── Message Communication ───────────────────────────────────────────────────

// SendMessage 追加一条消息到 messages.jsonl。
// msgType 应为 "finding" | "request" | "broadcast"。
// receiver 为特定 agent 名或 "all" 表示广播。
func (m *HandoffManager) SendMessage(sender, receiver, msgType, content string) error {
	msg := &HandoffMessage{
		ID:        uuid.NewString()[:8],
		Sender:    sender,
		Receiver:  receiver,
		MsgType:   msgType,
		Content:   content,
		CreatedAt: time.Now().Unix(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return appendLine(m.messagesPath(), data)
}

// ReadMessages 返回发给 receiver 的消息（包括广播消息）。
func (m *HandoffManager) ReadMessages(receiver string) ([]*HandoffMessage, error) {
	all, err := readJSONL[HandoffMessage](m.messagesPath())
	if err != nil {
		return nil, err
	}
	var result []*HandoffMessage
	for _, msg := range all {
		if msg.Receiver == "all" || msg.Receiver == receiver {
			result = append(result, msg)
		}
	}
	return result, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (m *HandoffManager) appendTask(task *HandoffTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	return appendLine(m.tasksPath(), data)
}

// writeTasks rewrites the entire tasks.jsonl file with updated task list.
func (m *HandoffManager) writeTasks(tasks []*HandoffTask) error {
	w, err := fileop.CreateFileAtomic(m.tasksPath(), 0o644)
	if err != nil {
		return fmt.Errorf("open tasks file: %w", err)
	}
	enc := json.NewEncoder(w)
	for _, t := range tasks {
		if err := enc.Encode(t); err != nil {
			w.Abort()
			return fmt.Errorf("encode task: %w", err)
		}
	}
	return w.Close()
}

// readJSONL reads a JSONL file and unmarshals each line into T.
// Returns an empty slice (not an error) if the file does not exist.
func readJSONL[T any](path string) ([]*T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var items []*T
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, &item)
	}
	return items, scanner.Err()
}

// appendLine appends a JSON-encoded line to the given file, creating it if needed.
func appendLine(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
