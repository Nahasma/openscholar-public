package message_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/openscholar/openscholar/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/testutil"
)

func setupWithQuerier(t *testing.T) (session.Service, message.Service, db.Querier) {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	return session.NewService(q), message.NewService(q), q
}

func setup(t *testing.T) (session.Service, message.Service) {
	s, m, _ := setupWithQuerier(t)
	return s, m
}

func TestMessageService_CreateAndGet(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	msg, err := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Hello world"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, sess.ID, msg.SessionID)
	assert.Equal(t, message.User, msg.Role)
	assert.Equal(t, "Hello world", msg.Content().Text)

	got, err := messages.Get(ctx, msg.ID)
	require.NoError(t, err)
	assert.Equal(t, msg.ID, got.ID)
	assert.Equal(t, "Hello world", got.Content().Text)
}

func TestMessageService_Create_UserAutoFinish(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg, _ := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "test"}},
	})
	// User messages auto-get a Finish part
	assert.True(t, msg.IsFinished())
}

func TestMessageService_Create_AssistantNoAutoFinish(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg, _ := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "response"}},
	})
	// Assistant messages do NOT auto-get a Finish part
	assert.False(t, msg.IsFinished())
}

func TestMessageService_List(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestMessage(t, messages, sess.ID, message.User, "first")
	testutil.CreateTestMessage(t, messages, sess.ID, message.User, "second")
	testutil.CreateTestMessage(t, messages, sess.ID, message.User, "third")

	list, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestMessageService_List_IsolatedBySession(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess1 := testutil.CreateTestSession(t, sessions)
	sess2 := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestMessage(t, messages, sess1.ID, message.User, "session1 msg")
	testutil.CreateTestMessage(t, messages, sess2.ID, message.User, "session2 msg")

	list1, _ := messages.List(ctx, sess1.ID)
	list2, _ := messages.List(ctx, sess2.ID)
	assert.Len(t, list1, 1)
	assert.Len(t, list2, 1)
}

func TestMessageService_Update(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg, _ := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "initial"}},
	})

	msg.Parts = []message.ContentPart{
		message.TextContent{Text: "updated"},
		message.Finish{Reason: message.FinishReasonEndTurn},
	}
	err := messages.Update(ctx, msg)
	require.NoError(t, err)

	got, _ := messages.Get(ctx, msg.ID)
	assert.Equal(t, "updated", got.Content().Text)
	assert.True(t, got.IsFinished())
}

func TestMessageService_Delete(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg := testutil.CreateTestMessage(t, messages, sess.ID, message.User, "to delete")
	err := messages.Delete(ctx, msg.ID)
	require.NoError(t, err)

	_, err = messages.Get(ctx, msg.ID)
	assert.Error(t, err)
}

func TestMessageService_DeleteSessionMessages(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestMessage(t, messages, sess.ID, message.User, "msg1")
	testutil.CreateTestMessage(t, messages, sess.ID, message.User, "msg2")

	err := messages.DeleteSessionMessages(ctx, sess.ID)
	require.NoError(t, err)

	list, _ := messages.List(ctx, sess.ID)
	assert.Empty(t, list)
}

func TestMessageService_ToolCallRoundTrip(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg, err := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Let me check"},
			message.ToolCall{ID: "tc1", Name: "view", Input: `{"path":"test.go"}`, Finished: true},
		},
	})
	require.NoError(t, err)

	got, _ := messages.Get(ctx, msg.ID)
	tcs := got.ToolCalls()
	require.Len(t, tcs, 1)
	assert.Equal(t, "tc1", tcs[0].ID)
	assert.Equal(t, "view", tcs[0].Name)
}

func TestMessageService_PubSub(t *testing.T) {
	sessions, messages := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := testutil.CreateTestSession(t, sessions)
	ch := messages.Subscribe(ctx)

	created := testutil.CreateTestMessage(t, messages, sess.ID, message.User, "pubsub test")

	event := <-ch
	assert.Equal(t, pubsub.CreatedEvent, event.Type)
	assert.Equal(t, created.ID, event.Payload.ID)
}

func TestMessageService_UsageMetaRoundTrip(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	msg, err := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "hello"}},
		Usage: message.Usage{
			InputTokens:              1200,
			OutputTokens:             240,
			CacheCreationInputTokens: 900,
			CacheReadInputTokens:     300,
		},
		Meta: map[string]any{
			"compact_boundary": true,
			"source":           "test",
		},
	})
	require.NoError(t, err)

	got, err := messages.Get(ctx, msg.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1200), got.Usage.InputTokens)
	assert.Equal(t, int64(240), got.Usage.OutputTokens)
	assert.Equal(t, int64(900), got.Usage.CacheCreationInputTokens)
	assert.Equal(t, int64(300), got.Usage.CacheReadInputTokens)
	assert.Equal(t, true, got.Meta["compact_boundary"])
	assert.Equal(t, "test", got.Meta["source"])

	got.Usage.InputTokens = 1300
	got.Meta["source"] = "updated"
	require.NoError(t, messages.Update(ctx, got))

	got2, err := messages.Get(ctx, msg.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1300), got2.Usage.InputTokens)
	assert.Equal(t, "updated", got2.Meta["source"])
}

func TestMessageService_InvalidMetaFallback(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	created, err := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "meta"}},
	})
	require.NoError(t, err)

	err = q.UpdateMessage(ctx, db.UpdateMessageParams{
		ID:                       created.ID,
		Parts:                    `[{"type":"text","data":{"text":"meta"}}]`,
		SearchText:               "meta",
		FinishedAt:               sql.NullInt64{},
		InputTokens:              0,
		OutputTokens:             0,
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
		Meta:                     "{invalid-json",
	})
	require.NoError(t, err)

	got, err := messages.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, got.Meta)
	assert.Empty(t, got.Meta)
}

func TestMessageService_SearchTextPersisted(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	created, err := messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "plain text"},
			message.ReasoningContent{Thinking: "reasoning line"},
			message.ToolCall{ID: "tc1", Name: "Read", Input: `{"path":"a.go"}`},
			message.ToolResult{ToolCallID: "tc1", Name: "Read", Content: "file body"},
		},
	})
	require.NoError(t, err)

	row, err := q.GetMessage(ctx, created.ID)
	require.NoError(t, err)
	assert.Contains(t, row.SearchText, "plain text")
	assert.Contains(t, row.SearchText, "reasoning line")
	assert.Contains(t, row.SearchText, "Read")
	assert.Contains(t, row.SearchText, "file body")
}

func TestMessageService_BackfillSearchTextMarksUnparseableRows(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	_, err := q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "a-invalid",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      "{invalid-json",
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)
	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "b-valid",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"second searchable"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)

	backfiller, ok := messages.(interface {
		BackfillSearchText(context.Context, int) (int, error)
	})
	require.True(t, ok)

	count, err := backfiller.BackfillSearchText(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	row, err := q.GetMessage(ctx, "a-invalid")
	require.NoError(t, err)
	assert.Equal(t, "[unparseable]", row.SearchText)

	count, err = backfiller.BackfillSearchText(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	row, err = q.GetMessage(ctx, "b-valid")
	require.NoError(t, err)
	assert.Contains(t, row.SearchText, "second searchable")
}

func TestMessageService_BackfillSearchTextForResumeSearchTargetsQuery(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)
	sess.ProjectPath = "/tmp/resume-target"
	_, err := sessions.Save(ctx, sess)
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "old-noise",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"foo unrelated backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)
	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "old-target",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"targeted foo, bar backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)

	backfiller, ok := messages.(interface {
		BackfillSearchTextForResumeSearch(context.Context, string, string, int) (int, error)
	})
	require.True(t, ok)

	count, err := backfiller.BackfillSearchTextForResumeSearch(ctx, "/tmp/resume-target", "foo bar", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	target, err := q.GetMessage(ctx, "old-target")
	require.NoError(t, err)
	assert.Contains(t, target.SearchText, "targeted foo, bar backlog")
	noise, err := q.GetMessage(ctx, "old-noise")
	require.NoError(t, err)
	assert.Empty(t, noise.SearchText)
}

func TestMessageService_BackfillSearchTextForResumeSearchTreatsUnderscoreLiterally(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)
	sess.ProjectPath = "/tmp/resume-underscore"
	_, err := sessions.Save(ctx, sess)
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "underscore-noise",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"fooXbar backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)
	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "underscore-target",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"Foo_Bar backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)

	backfiller, ok := messages.(interface {
		BackfillSearchTextForResumeSearch(context.Context, string, string, int) (int, error)
	})
	require.True(t, ok)

	count, err := backfiller.BackfillSearchTextForResumeSearch(ctx, "/tmp/resume-underscore", "foo_bar", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	target, err := q.GetMessage(ctx, "underscore-target")
	require.NoError(t, err)
	assert.Contains(t, target.SearchText, "Foo_Bar backlog")
	noise, err := q.GetMessage(ctx, "underscore-noise")
	require.NoError(t, err)
	assert.Empty(t, noise.SearchText)
}

func TestMessageService_BackfillSearchTextForResumeSearchTreatsPercentLiterally(t *testing.T) {
	sessions, messages, q := setupWithQuerier(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)
	sess.ProjectPath = "/tmp/resume-percent"
	_, err := sessions.Save(ctx, sess)
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "percent-noise",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"fooXbar backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)
	_, err = q.CreateMessage(ctx, db.CreateMessageParams{
		ID:         "percent-target",
		SessionID:  sess.ID,
		Role:       string(message.User),
		Parts:      `[{"type":"text","data":{"text":"Foo%Bar backlog"}}]`,
		SearchText: "",
		Meta:       "{}",
	})
	require.NoError(t, err)

	backfiller, ok := messages.(interface {
		BackfillSearchTextForResumeSearch(context.Context, string, string, int) (int, error)
	})
	require.True(t, ok)

	count, err := backfiller.BackfillSearchTextForResumeSearch(ctx, "/tmp/resume-percent", "foo%bar", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	target, err := q.GetMessage(ctx, "percent-target")
	require.NoError(t, err)
	assert.Contains(t, target.SearchText, "Foo%Bar backlog")
	noise, err := q.GetMessage(ctx, "percent-noise")
	require.NoError(t, err)
	assert.Empty(t, noise.SearchText)
}
