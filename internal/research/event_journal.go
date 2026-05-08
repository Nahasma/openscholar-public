package research

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

// 最小事件集（6 种）
const (
	JournalPipelineCreated  = "pipeline.created"
	JournalPhaseStarted     = "phase.started"
	JournalPhaseCompleted   = "phase.completed"
	JournalPhaseFailed      = "phase.failed"
	JournalBudgetWarning    = "budget.warning"
	JournalBudgetExceeded   = "budget.exceeded"
	JournalCompactTriggered = "compact.triggered"
	JournalWorktreeCreated  = "worktree.created"
)

// PipelineEvent 审计日志事件
type PipelineEvent struct {
	ID         int64  `json:"id"`
	PipelineID string `json:"pipeline_id"`
	EventType  string `json:"event_type"`
	Payload    string `json:"payload"`
	CreatedAt  int64  `json:"created_at"`
}

// EventJournal 将 ResearchEvent 持久化到 pipeline_events 表
type EventJournal struct {
	store *Store
}

// NewEventJournal 创建 EventJournal
func NewEventJournal(store *Store) *EventJournal {
	return &EventJournal{store: store}
}

// Emit 记录一条事件到数据库
func (j *EventJournal) Emit(pipelineID, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	evt := &PipelineEvent{
		PipelineID: pipelineID,
		EventType:  eventType,
		Payload:    string(data),
		CreatedAt:  time.Now().Unix(),
	}
	return j.store.CreatePipelineEvent(evt)
}

// ListRecent 返回某 pipeline 最近 limit 条事件
func (j *EventJournal) ListRecent(pipelineID string, limit int) ([]*PipelineEvent, error) {
	return j.store.ListRecentEvents(pipelineID, limit)
}

// ListByType 返回某 pipeline 特定类型的所有事件
func (j *EventJournal) ListByType(pipelineID, eventType string) ([]*PipelineEvent, error) {
	return j.store.ListEventsByType(pipelineID, eventType)
}

// SubscribeBroker 订阅 ResearchEvent Broker，将事件映射并持久化
func (j *EventJournal) SubscribeBroker(ctx context.Context, broker *pubsub.Broker[ResearchEvent]) {
	ch := broker.Subscribe(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				re := evt.Payload
				journalType := mapResearchEventToJournal(re.Type)
				if journalType == "" {
					continue
				}
				_ = j.Emit(re.PipelineID, journalType, map[string]any{
					"phase_id": re.PhaseID,
					"data":     re.Data,
				})
			}
		}
	}()
}

// mapResearchEventToJournal 将 ResearchEventType 映射到 journal 常量
func mapResearchEventToJournal(t ResearchEventType) string {
	switch t {
	case EventPhaseStarted:
		return JournalPhaseStarted
	case EventPhaseCompleted:
		return JournalPhaseCompleted
	case EventPhaseFailed:
		return JournalPhaseFailed
	case EventBudgetWarning:
		return JournalBudgetWarning
	case EventBudgetExceeded:
		return JournalBudgetExceeded
	default:
		return ""
	}
}
