package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

type CreateMessageParams struct {
	Role  MessageRole
	Parts []ContentPart
	Model models.ModelID
	Usage Usage
	Meta  map[string]any
}

type Service interface {
	pubsub.Subscriber[Message]
	Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error)
	Update(ctx context.Context, message Message) error
	Get(ctx context.Context, id string) (Message, error)
	List(ctx context.Context, sessionID string) ([]Message, error)
	Delete(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error
}

type service struct {
	*pubsub.Broker[Message]
	q db.Querier
}

func NewService(q db.Querier) Service {
	return &service{
		Broker: pubsub.NewBroker[Message](),
		q:      q,
	}
}

func (s *service) Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error) {
	if params.Role != Assistant {
		params.Parts = append(params.Parts, Finish{Reason: "stop"})
	}
	partsJSON, err := marshallParts(params.Parts)
	if err != nil {
		return Message{}, err
	}
	metaJSON, err := marshalMeta(params.Meta)
	if err != nil {
		return Message{}, err
	}
	dbMessage, err := s.q.CreateMessage(ctx, db.CreateMessageParams{
		ID:                       uuid.New().String(),
		SessionID:                sessionID,
		Role:                     string(params.Role),
		Parts:                    string(partsJSON),
		SearchText:               SearchText(params.Parts),
		Model:                    sql.NullString{String: string(params.Model), Valid: true},
		InputTokens:              params.Usage.InputTokens,
		OutputTokens:             params.Usage.OutputTokens,
		CacheCreationInputTokens: params.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     params.Usage.CacheReadInputTokens,
		Meta:                     string(metaJSON),
	})
	if err != nil {
		return Message{}, err
	}
	if params.Role == User {
		s.maybeUpdateFirstPrompt(ctx, sessionID, params.Parts)
	}
	message, err := s.fromDBItem(dbMessage)
	if err != nil {
		return Message{}, err
	}
	s.Publish(pubsub.CreatedEvent, message)
	return message, nil
}

func (s *service) Update(ctx context.Context, message Message) error {
	parts, err := marshallParts(message.Parts)
	if err != nil {
		return err
	}
	metaJSON, err := marshalMeta(message.Meta)
	if err != nil {
		return err
	}
	finishedAt := sql.NullInt64{}
	if f := message.FinishPart(); f != nil {
		finishedAt.Int64 = f.Time
		finishedAt.Valid = true
	}
	err = s.q.UpdateMessage(ctx, db.UpdateMessageParams{
		ID:                       message.ID,
		Parts:                    string(parts),
		SearchText:               SearchText(message.Parts),
		FinishedAt:               finishedAt,
		InputTokens:              message.Usage.InputTokens,
		OutputTokens:             message.Usage.OutputTokens,
		CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		Meta:                     string(metaJSON),
	})
	if err != nil {
		return err
	}
	message.UpdatedAt = time.Now().Unix()
	s.Publish(pubsub.UpdatedEvent, message)
	return nil
}

func (s *service) maybeUpdateFirstPrompt(ctx context.Context, sessionID string, parts []ContentPart) {
	text := strings.TrimSpace(SearchText(parts))
	if text == "" {
		return
	}
	sess, err := s.q.GetSessionByID(ctx, sessionID)
	if err != nil || strings.TrimSpace(sess.FirstPrompt.String) != "" {
		return
	}
	rootID := sess.RootSessionID
	if rootID == "" {
		rootID = sess.ID
	}
	_, _ = s.q.UpdateSession(ctx, db.UpdateSessionParams{
		ID:                  sess.ID,
		Title:               sess.Title,
		PromptTokens:        sess.PromptTokens,
		CompletionTokens:    sess.CompletionTokens,
		SummaryMessageID:    sess.SummaryMessageID,
		Cost:                sess.Cost,
		Summary:             sess.Summary,
		Tags:                sess.Tags,
		FirstPrompt:         sql.NullString{String: text, Valid: text != ""},
		GitBranch:           sess.GitBranch,
		ProjectPath:         sess.ProjectPath,
		WorktreePath:        sess.WorktreePath,
		Mode:                sess.Mode,
		RootSessionID:       rootID,
		ForkedFromSessionID: sess.ForkedFromSessionID,
	})
}

func SearchText(parts []ContentPart) string {
	var chunks []string
	for _, part := range parts {
		switch p := part.(type) {
		case TextContent:
			chunks = append(chunks, p.Text)
		case ReasoningContent:
			chunks = append(chunks, p.Thinking)
		case ToolCall:
			chunks = append(chunks, p.Name, p.Input)
		case ToolResult:
			chunks = append(chunks, p.Name, p.Content)
		}
	}
	return strings.TrimSpace(strings.Join(chunks, "\n"))
}

func (s *service) Get(ctx context.Context, id string) (Message, error) {
	dbMessage, err := s.q.GetMessage(ctx, id)
	if err != nil {
		return Message{}, err
	}
	return s.fromDBFields(
		dbMessage.ID, dbMessage.SessionID, dbMessage.Role, dbMessage.Parts, dbMessage.Model,
		dbMessage.CreatedAt, dbMessage.UpdatedAt, dbMessage.FinishedAt,
		dbMessage.InputTokens, dbMessage.OutputTokens, dbMessage.CacheCreationInputTokens, dbMessage.CacheReadInputTokens, dbMessage.Meta,
	)
}

func (s *service) List(ctx context.Context, sessionID string) ([]Message, error) {
	dbMessages, err := s.q.ListMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	messages := make([]Message, len(dbMessages))
	for i, dbMessage := range dbMessages {
		messages[i], err = s.fromDBFields(
			dbMessage.ID, dbMessage.SessionID, dbMessage.Role, dbMessage.Parts, dbMessage.Model,
			dbMessage.CreatedAt, dbMessage.UpdatedAt, dbMessage.FinishedAt,
			dbMessage.InputTokens, dbMessage.OutputTokens, dbMessage.CacheCreationInputTokens, dbMessage.CacheReadInputTokens, dbMessage.Meta,
		)
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	message, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	err = s.q.DeleteMessage(ctx, message.ID)
	if err != nil {
		return err
	}
	s.Publish(pubsub.DeletedEvent, message)
	return nil
}

func (s *service) DeleteSessionMessages(ctx context.Context, sessionID string) error {
	return s.q.DeleteSessionMessages(ctx, sessionID)
}

func (s *service) BackfillSearchText(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.q.ListMessagesMissingSearchText(ctx, int64(limit))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		ok, err := s.backfillSearchTextRow(ctx,
			row.ID, row.SessionID, row.Role, row.Parts, row.Model,
			row.CreatedAt, row.UpdatedAt, row.FinishedAt,
			row.InputTokens, row.OutputTokens, row.CacheCreationInputTokens, row.CacheReadInputTokens, row.Meta,
		)
		if err != nil {
			return count, err
		}
		if ok {
			count++
		}
	}
	return count, nil
}

func (s *service) BackfillSearchTextForResumeSearch(ctx context.Context, projectPath, query string, limit int) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, nil
	}
	if limit <= 0 {
		limit = 500
	}
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return 0, nil
	}
	var tokenArgs [5]string
	for i := 0; i < len(tokens) && i < len(tokenArgs); i++ {
		tokenArgs[i] = tokens[i]
	}
	rows, err := s.q.ListMessagesMissingSearchTextForResumeSearch(ctx, db.ListMessagesMissingSearchTextForResumeSearchParams{
		Token1:      tokenArgs[0],
		Token2:      tokenArgs[1],
		Token3:      tokenArgs[2],
		Token4:      tokenArgs[3],
		Token5:      tokenArgs[4],
		ProjectPath: projectPath,
		Limit:       int64(limit),
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		ok, err := s.backfillSearchTextRow(ctx,
			row.ID, row.SessionID, row.Role, row.Parts, row.Model,
			row.CreatedAt, row.UpdatedAt, row.FinishedAt,
			row.InputTokens, row.OutputTokens, row.CacheCreationInputTokens, row.CacheReadInputTokens, row.Meta,
		)
		if err != nil {
			return count, err
		}
		if ok {
			count++
		}
	}
	return count, nil
}

func (s *service) backfillSearchTextRow(
	ctx context.Context,
	id string,
	sessionID string,
	role string,
	partsRaw string,
	model sql.NullString,
	createdAt int64,
	updatedAt int64,
	finishedAt sql.NullInt64,
	inputTokens int64,
	outputTokens int64,
	cacheCreationInputTokens int64,
	cacheReadInputTokens int64,
	metaRaw string,
) (bool, error) {
	msg, err := s.fromDBFields(
		id, sessionID, role, partsRaw, model,
		createdAt, updatedAt, finishedAt,
		inputTokens, outputTokens, cacheCreationInputTokens, cacheReadInputTokens, metaRaw,
	)
	text := ""
	if err != nil {
		text = "[unparseable]"
	} else {
		text = SearchText(msg.Parts)
		if text == "" {
			text = "[empty]"
		}
	}
	err = s.q.BackfillMessageSearchText(ctx, db.BackfillMessageSearchTextParams{
		SearchText: text,
		ID:         id,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *service) fromDBItem(item db.Message) (Message, error) {
	return s.fromDBFields(
		item.ID, item.SessionID, item.Role, item.Parts, item.Model,
		item.CreatedAt, item.UpdatedAt, item.FinishedAt,
		item.InputTokens, item.OutputTokens, item.CacheCreationInputTokens, item.CacheReadInputTokens, item.Meta,
	)
}

func (s *service) fromDBFields(
	id string,
	sessionID string,
	role string,
	partsRaw string,
	model sql.NullString,
	createdAt int64,
	updatedAt int64,
	finishedAt sql.NullInt64,
	inputTokens int64,
	outputTokens int64,
	cacheCreationInputTokens int64,
	cacheReadInputTokens int64,
	metaRaw string,
) (Message, error) {
	parts, err := unmarshallParts([]byte(partsRaw))
	if err != nil {
		return Message{}, err
	}
	meta := unmarshalMeta([]byte(metaRaw))
	// Recover Finish part from finished_at if parts lost it (e.g. crash during batch persist)
	if finishedAt.Valid {
		hasFinish := false
		for _, p := range parts {
			if _, ok := p.(Finish); ok {
				hasFinish = true
				break
			}
		}
		if !hasFinish {
			parts = append(parts, Finish{
				Reason: FinishReasonUnknown,
				Time:   finishedAt.Int64,
			})
		}
	}
	return Message{
		ID:        id,
		SessionID: sessionID,
		Role:      MessageRole(role),
		Parts:     parts,
		Model:     models.ModelID(model.String),
		Usage: Usage{
			InputTokens:              inputTokens,
			OutputTokens:             outputTokens,
			CacheCreationInputTokens: cacheCreationInputTokens,
			CacheReadInputTokens:     cacheReadInputTokens,
		},
		Meta:      meta,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

type partType string

const (
	reasoningType  partType = "reasoning"
	textType       partType = "text"
	binaryType     partType = "binary"
	toolCallType   partType = "tool_call"
	toolResultType partType = "tool_result"
	finishType     partType = "finish"
)

type partWrapper struct {
	Type partType    `json:"type"`
	Data ContentPart `json:"data"`
}

func marshallParts(parts []ContentPart) ([]byte, error) {
	wrappedParts := make([]partWrapper, len(parts))
	for i, part := range parts {
		var typ partType
		switch part.(type) {
		case ReasoningContent:
			typ = reasoningType
		case TextContent:
			typ = textType
		case BinaryContent:
			typ = binaryType
		case ToolCall:
			typ = toolCallType
		case ToolResult:
			typ = toolResultType
		case Finish:
			typ = finishType
		default:
			return nil, fmt.Errorf("unknown part type: %T", part)
		}
		wrappedParts[i] = partWrapper{Type: typ, Data: part}
	}
	return json.Marshal(wrappedParts)
}

func unmarshallParts(data []byte) ([]ContentPart, error) {
	var temp []json.RawMessage
	if err := json.Unmarshal(data, &temp); err != nil {
		return nil, err
	}

	parts := make([]ContentPart, 0, len(temp))
	for _, rawPart := range temp {
		var wrapper struct {
			Type partType        `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rawPart, &wrapper); err != nil {
			return nil, err
		}

		switch wrapper.Type {
		case reasoningType:
			var part ReasoningContent
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case textType:
			var part TextContent
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case binaryType:
			var part BinaryContent
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case toolCallType:
			var part ToolCall
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				// Skip corrupted tool call parts instead of failing
				continue
			}
			parts = append(parts, part)
		case toolResultType:
			var part ToolResult
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				// Skip corrupted tool result parts instead of failing
				continue
			}
			parts = append(parts, part)
		case finishType:
			var part Finish
			if err := json.Unmarshal(wrapper.Data, &part); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		default:
			return nil, fmt.Errorf("unknown part type: %s", wrapper.Type)
		}
	}
	return parts, nil
}
