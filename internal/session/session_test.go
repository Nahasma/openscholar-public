package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func newService(t *testing.T) session.Service {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	return session.NewService(q)
}

func TestSessionService_CreateAndGet(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	sess, err := svc.Create(ctx, "My Session")
	require.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "My Session", sess.Title)
	assert.NotZero(t, sess.CreatedAt)

	got, err := svc.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)
	assert.Equal(t, sess.Title, got.Title)
}

func TestSessionService_Get_NotFound(t *testing.T) {
	svc := newService(t)
	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestSessionService_List(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.Create(ctx, "First")
	svc.Create(ctx, "Second")
	svc.Create(ctx, "Third")

	list, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestSessionService_List_Empty(t *testing.T) {
	svc := newService(t)
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestSessionService_Save(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	sess, _ := svc.Create(ctx, "Original")
	sess.Title = "Updated"
	sess.PromptTokens = 500
	sess.CompletionTokens = 200
	sess.Cost = 0.05

	saved, err := svc.Save(ctx, sess)
	require.NoError(t, err)
	assert.Equal(t, "Updated", saved.Title)
	assert.Equal(t, int64(500), saved.PromptTokens)
	assert.Equal(t, int64(200), saved.CompletionTokens)
	assert.InDelta(t, 0.05, saved.Cost, 0.001)

	got, _ := svc.Get(ctx, sess.ID)
	assert.Equal(t, "Updated", got.Title)
}

func TestSessionService_Delete(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	sess, _ := svc.Create(ctx, "To Delete")
	err := svc.Delete(ctx, sess.ID)
	require.NoError(t, err)

	_, err = svc.Get(ctx, sess.ID)
	assert.Error(t, err)
}

func TestSessionService_Delete_NotFound(t *testing.T) {
	svc := newService(t)
	err := svc.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestSessionService_CreateTaskSession(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	parent, _ := svc.Create(ctx, "Parent")
	child, err := svc.CreateTaskSession(ctx, "child-1", parent.ID, "Task")
	require.NoError(t, err)

	assert.Equal(t, "child-1", child.ID)
	assert.Equal(t, parent.ID, child.ParentSessionID)
	assert.Equal(t, "Task", child.Title)

	got, _ := svc.Get(ctx, child.ID)
	assert.Equal(t, parent.ID, got.ParentSessionID)
}

func TestSessionService_CreateTitleSession(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	parent, _ := svc.Create(ctx, "Parent")
	title, err := svc.CreateTitleSession(ctx, parent.ID)
	require.NoError(t, err)

	assert.Equal(t, "title-"+parent.ID, title.ID)
	assert.Equal(t, parent.ID, title.ParentSessionID)
}

func TestSessionService_PubSub_Create(t *testing.T) {
	svc := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := svc.Subscribe(ctx)

	sess, _ := svc.Create(ctx, "PubSub Test")

	event := <-ch
	assert.Equal(t, pubsub.CreatedEvent, event.Type)
	assert.Equal(t, sess.ID, event.Payload.ID)
}

func TestSessionService_PubSub_Update(t *testing.T) {
	svc := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, _ := svc.Create(ctx, "Before Update")
	ch := svc.Subscribe(ctx)

	sess.Title = "After Update"
	svc.Save(ctx, sess)

	event := <-ch
	assert.Equal(t, pubsub.UpdatedEvent, event.Type)
	assert.Equal(t, "After Update", event.Payload.Title)
}

func TestSessionService_PubSub_Delete(t *testing.T) {
	svc := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, _ := svc.Create(ctx, "To Delete")
	ch := svc.Subscribe(ctx)

	svc.Delete(ctx, sess.ID)

	event := <-ch
	assert.Equal(t, pubsub.DeletedEvent, event.Type)
	assert.Equal(t, sess.ID, event.Payload.ID)
}
