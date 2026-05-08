package permission_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestPermission_AutoApproveSession(t *testing.T) {
	svc := permission.NewPermissionService()
	svc.AutoApproveSession("sess-1")

	granted := svc.Request(permission.CreatePermissionRequest{
		SessionID: "sess-1",
		ToolName:  "edit",
		Action:    "write",
		Path:      "/tmp/test",
	})
	assert.True(t, granted)
}

func TestPermission_NoAutoApprove_Blocks(t *testing.T) {
	svc := permission.NewPermissionService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	// Request will block until Grant/Deny — handle in goroutine
	done := make(chan bool, 1)
	go func() {
		result := svc.Request(permission.CreatePermissionRequest{
			SessionID: "sess-2",
			ToolName:  "bash",
			Action:    "execute",
			Path:      "/tmp",
		})
		done <- result
	}()

	// Wait for the permission request event
	select {
	case event := <-ch:
		assert.Equal(t, pubsub.CreatedEvent, event.Type)
		svc.Grant(event.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for permission event")
	}

	select {
	case result := <-done:
		assert.True(t, result)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for grant result")
	}
}

func TestPermission_Deny(t *testing.T) {
	svc := permission.NewPermissionService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	done := make(chan bool, 1)
	go func() {
		result := svc.Request(permission.CreatePermissionRequest{
			SessionID: "sess-3",
			ToolName:  "bash",
			Action:    "execute",
			Path:      "/tmp",
		})
		done <- result
	}()

	event := <-ch
	svc.Deny(event.Payload)

	select {
	case result := <-done:
		assert.False(t, result)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPermission_RemoveAutoApproveSession(t *testing.T) {
	svc := permission.NewPermissionService()
	svc.AutoApproveSession("sess-4")
	svc.RemoveAutoApproveSession("sess-4")

	// Now should block — test by granting from goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	done := make(chan bool, 1)
	go func() {
		result := svc.Request(permission.CreatePermissionRequest{
			SessionID: "sess-4",
			ToolName:  "edit",
			Action:    "write",
			Path:      "/tmp",
		})
		done <- result
	}()

	event := <-ch
	svc.Grant(event.Payload)

	select {
	case result := <-done:
		assert.True(t, result) // Granted manually
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPermission_RequirePromptSessionBypassesBuiltInWriteAllow(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	workspace := t.TempDir()
	svc := permission.NewPermissionServiceWithDB(q, workspace)
	svc.RequirePromptSession("child")

	read := svc.RequestDecision(permission.CreatePermissionRequest{
		SessionID: "child",
		ToolName:  "View",
		Action:    "read",
		Path:      workspace + "/README.md",
	})
	if !read.Allowed {
		t.Fatalf("expected read request to keep using built-in allow rules: %#v", read)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	done := make(chan bool, 1)
	go func() {
		done <- svc.Request(permission.CreatePermissionRequest{
			SessionID: "child",
			ToolName:  "Write",
			Action:    "write",
			Path:      workspace + "/out.txt",
		})
	}()

	select {
	case event := <-ch:
		assert.Equal(t, pubsub.CreatedEvent, event.Type)
		assert.Equal(t, "Write", event.Payload.ToolName)
		svc.Grant(event.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for required prompt")
	}

	select {
	case result := <-done:
		assert.True(t, result)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for grant result")
	}

	svc.RemoveRequirePromptSession("child")
	write := svc.RequestDecision(permission.CreatePermissionRequest{
		SessionID: "child",
		ToolName:  "Write",
		Action:    "write",
		Path:      workspace + "/out.txt",
	})
	if !write.Allowed {
		t.Fatalf("expected built-in write allow after prompt requirement removal: %#v", write)
	}
}

func TestPermission_GrantPersistent(t *testing.T) {
	svc := permission.NewPermissionService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	// First request: grant persistent
	go func() {
		svc.Request(permission.CreatePermissionRequest{
			SessionID: "sess-5",
			ToolName:  "edit",
			Action:    "write",
			Path:      "/tmp/project",
		})
	}()

	event := <-ch
	svc.GrantPersistent(event.Payload)

	// Second request with same tool/action/session/path should auto-approve
	time.Sleep(10 * time.Millisecond) // ensure first grant is processed
	granted := svc.Request(permission.CreatePermissionRequest{
		SessionID: "sess-5",
		ToolName:  "edit",
		Action:    "write",
		Path:      "/tmp/project",
	})
	assert.True(t, granted)
}

func TestPermission_GrantAlways_WithDB(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	svc := permission.NewPermissionServiceWithDB(q, "/tmp")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := svc.Subscribe(ctx)

	go func() {
		svc.Request(permission.CreatePermissionRequest{
			SessionID: "sess-6",
			ToolName:  "bash",
			Action:    "execute",
			Path:      "/tmp/scripts",
		})
	}()

	event := <-ch
	svc.GrantAlways(event.Payload)

	// Should auto-approve for any session with same tool/action/path
	time.Sleep(10 * time.Millisecond)
	granted := svc.Request(permission.CreatePermissionRequest{
		SessionID: "sess-7", // different session
		ToolName:  "bash",
		Action:    "execute",
		Path:      "/tmp/scripts",
	})
	assert.True(t, granted)
}
