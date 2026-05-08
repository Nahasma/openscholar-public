// Package sdk provides a headless (non-TUI) API for programmatic access to OpenScholar.
package sdk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/llm/agent"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/message"
)

// Options configures an SDK session.
type Options struct {
	CWD         string          // working directory
	ConfigPath  string          // config file path (optional)
	Model       models.ModelID  // model (optional, defaults to config)
	MaxTurns    int             // max tool call rounds (default 25)
	Timeout     time.Duration   // total timeout (default 5m)
	AutoApprove bool            // auto-approve tool execution (default false)
}

// Session is a headless OpenScholar session.
type Session struct {
	app       *app.App
	conn      *sql.DB
	sessionID string
}

// QueryResult holds the result of a headless query.
type QueryResult struct {
	SessionID    string
	Messages     []message.Message
	FinalText    string
	Usage        provider.TokenUsage
	ToolsInvoked []string
}

// New creates a new headless SDK session.
func New(ctx context.Context, opts Options) (*Session, error) {
	if opts.CWD != "" {
		config.SetWorkingDirectory(opts.CWD)
	}
	cwd := config.WorkingDirectory()
	if _, err := config.Load(cwd); err != nil {
		return nil, fmt.Errorf("sdk: failed to load config: %w", err)
	}
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 25
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}

	conn, err := db.Connect()
	if err != nil {
		return nil, fmt.Errorf("sdk: failed to open database: %w", err)
	}

	application, err := app.New(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sdk: failed to create app: %w", err)
	}

	if opts.Model != "" {
		if err := application.SetModel(opts.Model); err != nil {
			conn.Close()
			return nil, fmt.Errorf("sdk: failed to set model: %w", err)
		}
	}

	sess, err := application.Sessions.Create(ctx, "SDK Session")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sdk: failed to create session: %w", err)
	}

	if opts.AutoApprove {
		application.Permissions.AutoApproveSession(sess.ID)
	}

	return &Session{
		app:       application,
		conn:      conn,
		sessionID: sess.ID,
	}, nil
}

// Query sends a prompt and waits for the agent to complete.
func (s *Session) Query(ctx context.Context, prompt string) (QueryResult, error) {
	eventCh, err := s.app.CoderAgent.Run(ctx, s.sessionID, prompt)
	if err != nil {
		return QueryResult{}, fmt.Errorf("sdk: agent failed to start: %w", err)
	}

	result := <-eventCh
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
			return QueryResult{SessionID: s.sessionID}, nil
		}
		return QueryResult{}, fmt.Errorf("sdk: agent error: %w", result.Error)
	}

	// Collect messages and tools used
	msgs, _ := s.app.Messages.List(ctx, s.sessionID)
	toolsUsed := make(map[string]bool)
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if tc, ok := part.(message.ToolCall); ok {
				toolsUsed[tc.Name] = true
			}
		}
	}
	toolNames := make([]string, 0, len(toolsUsed))
	for name := range toolsUsed {
		toolNames = append(toolNames, name)
	}

	return QueryResult{
		SessionID:    s.sessionID,
		Messages:     msgs,
		FinalText:    result.Message.Content().String(),
		ToolsInvoked: toolNames,
	}, nil
}

// Fork creates a child session that inherits the parent's context.
func (s *Session) Fork(ctx context.Context, title string) (*Session, error) {
	childSess, err := s.app.Sessions.Create(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("sdk: fork failed: %w", err)
	}

	// Copy snapshot from parent if forked runner is available
	if s.app.ForkedRunner != nil {
		if snap, ok := s.app.ForkedRunner.LatestSnapshot(s.sessionID); ok {
			s.app.ForkedRunner.SaveSnapshot(childSess.ID, snap)
		}
	}

	return &Session{
		app:       s.app,
		conn:      s.conn,
		sessionID: childSess.ID,
	}, nil
}

// SessionID returns the current session ID.
func (s *Session) SessionID() string {
	return s.sessionID
}

// Close shuts down the session and releases resources.
func (s *Session) Close() error {
	s.app.Shutdown()
	return s.conn.Close()
}
