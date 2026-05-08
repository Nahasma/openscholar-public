package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type ResolveMode string

const (
	ResolveLatest ResolveMode = "latest"
	ResolveID     ResolveMode = "id"
	ResolveExact  ResolveMode = "exact"
	ResolveQuery  ResolveMode = "query"
)

type ResumeRequest struct {
	Mode        ResolveMode
	Selector    string
	ProjectPath string
	Limit       int
}

type ResumeResolveResult struct {
	Session    Session
	Ambiguous  []Session
	Found      bool
	NeedPicker bool
}

type ResumeLoadResult struct {
	Session           Session
	Messages          []message.Message
	ToolMessageByCall map[string]message.Message
}

type ResumeService interface {
	Resolve(ctx context.Context, req ResumeRequest) (ResumeResolveResult, error)
	List(ctx context.Context, projectPath string, limit int) ([]Session, error)
	Load(ctx context.Context, sessionID string) (ResumeLoadResult, error)
	NormalizeMessagesForResume(msgs []message.Message) []message.Message
	Search(ctx context.Context, projectPath, query string, limit int) ([]Session, error)
}

type searchTextBackfiller interface {
	BackfillSearchText(ctx context.Context, limit int) (int, error)
}

type resumeSearchTextBackfiller interface {
	BackfillSearchTextForResumeSearch(ctx context.Context, projectPath, query string, limit int) (int, error)
}

type resumeService struct {
	q        db.Querier
	sessions Service
	messages message.Service
}

func NewResumeService(q db.Querier, sessions Service, messages message.Service) ResumeService {
	return &resumeService{q: q, sessions: sessions, messages: messages}
}

func (s *resumeService) List(ctx context.Context, projectPath string, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListSessionsForResume(ctx, db.ListSessionsForResumeParams{
		ProjectPath: projectPath,
		Limit:       int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Session, len(rows))
	for i := range rows {
		out[i] = s.fromDB(rows[i])
	}
	return out, nil
}

func (s *resumeService) Resolve(ctx context.Context, req ResumeRequest) (ResumeResolveResult, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	selector := strings.TrimSpace(req.Selector)
	if req.Mode == ResolveLatest {
		list, err := s.List(ctx, req.ProjectPath, 1)
		if err != nil {
			return ResumeResolveResult{}, err
		}
		if len(list) == 0 {
			return ResumeResolveResult{}, nil
		}
		return ResumeResolveResult{Session: list[0], Found: true}, nil
	}
	if selector == "" {
		return ResumeResolveResult{NeedPicker: true}, nil
	}
	if req.Mode == ResolveID || req.Mode == ResolveExact || req.Mode == ResolveQuery {
		sess, err := s.sessions.Get(ctx, selector)
		if err == nil {
			sess, err = s.rootForResume(ctx, sess)
			if err == nil && (req.ProjectPath == "" || sess.ProjectPath == "" || sess.ProjectPath == req.ProjectPath) {
				return ResumeResolveResult{Session: sess, Found: true}, nil
			}
		}
		if req.Mode == ResolveID {
			return ResumeResolveResult{}, nil
		}
	}

	exacts, err := s.q.ResolveSessionByExactTitle(ctx, db.ResolveSessionByExactTitleParams{
		Title:       selector,
		ProjectPath: req.ProjectPath,
		Limit:       int64(req.Limit),
	})
	if err != nil {
		return ResumeResolveResult{}, err
	}
	if len(exacts) == 1 {
		return ResumeResolveResult{Session: s.fromDB(exacts[0]), Found: true}, nil
	}
	if len(exacts) > 1 {
		amb := make([]Session, len(exacts))
		for i := range exacts {
			amb[i] = s.fromDB(exacts[i])
		}
		return ResumeResolveResult{Ambiguous: amb}, nil
	}
	if req.Mode == ResolveExact {
		return ResumeResolveResult{}, nil
	}

	results, err := s.Search(ctx, req.ProjectPath, selector, req.Limit)
	if err != nil {
		return ResumeResolveResult{}, err
	}
	if len(results) == 1 {
		return ResumeResolveResult{Session: results[0], Found: true}, nil
	}
	if len(results) > 1 {
		return ResumeResolveResult{Ambiguous: results}, nil
	}
	return ResumeResolveResult{}, nil
}

func (s *resumeService) Search(ctx context.Context, projectPath, query string, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 20
	}
	q := sanitizeResumeQuery(query)
	if q == "" {
		return s.List(ctx, projectPath, limit)
	}
	if bf, ok := s.messages.(resumeSearchTextBackfiller); ok {
		_, _ = bf.BackfillSearchTextForResumeSearch(ctx, projectPath, q, 5000)
	} else if bf, ok := s.messages.(searchTextBackfiller); ok {
		_, _ = bf.BackfillSearchText(ctx, 5000)
	}

	rows, err := s.q.SearchSessionsByMetadata(ctx, db.SearchSessionsByMetadataParams{
		ProjectPath: projectPath,
		Query:       q,
		Limit:       int64(limit),
	})
	if err != nil {
		return nil, err
	}
	cands := make([]Session, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for i := range rows {
		sess := s.fromDB(rows[i])
		if _, ok := seen[sess.ID]; ok {
			continue
		}
		seen[sess.ID] = struct{}{}
		cands = append(cands, sess)
	}

	if len(cands) < limit {
		ftsIDs, err := s.q.SearchRootSessionsByMessageFTS(ctx, db.SearchRootSessionsByMessageFTSParams{
			Query:       toFTSQuery(q),
			ProjectPath: projectPath,
			Limit:       int64(limit),
		})
		if err == nil {
			for _, id := range ftsIDs {
				if _, ok := seen[id]; ok {
					continue
				}
				sess, gerr := s.sessions.Get(ctx, id)
				if gerr != nil {
					continue
				}
				seen[id] = struct{}{}
				cands = append(cands, sess)
			}
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].UpdatedAt > cands[j].UpdatedAt
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	return cands, nil
}

func (s *resumeService) Load(ctx context.Context, sessionID string) (ResumeLoadResult, error) {
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return ResumeLoadResult{}, err
	}
	sess, err = s.rootForResume(ctx, sess)
	if err != nil {
		return ResumeLoadResult{}, err
	}
	msgs, err := s.messages.List(ctx, sess.ID)
	if err != nil {
		return ResumeLoadResult{}, err
	}
	msgs = s.NormalizeMessagesForResume(msgs)
	tools := make(map[string]message.Message)
	for _, msg := range msgs {
		if msg.Role != message.Tool {
			continue
		}
		for _, tr := range msg.ToolResults() {
			tools[tr.ToolCallID] = msg
		}
	}
	return ResumeLoadResult{Session: sess, Messages: msgs, ToolMessageByCall: tools}, nil
}

func (s *resumeService) NormalizeMessagesForResume(msgs []message.Message) []message.Message {
	return normalizeProviderSafeMessages(msgs)
}

func (s *resumeService) rootForResume(ctx context.Context, sess Session) (Session, error) {
	current := sess
	seen := map[string]struct{}{current.ID: {}}
	for depth := 0; depth < 32; depth++ {
		nextID := strings.TrimSpace(current.RootSessionID)
		if nextID == "" || nextID == current.ID {
			nextID = strings.TrimSpace(current.ParentSessionID)
		}
		if nextID == "" || nextID == current.ID {
			return current, nil
		}
		if _, ok := seen[nextID]; ok {
			return current, nil
		}
		next, err := s.sessions.Get(ctx, nextID)
		if err != nil {
			return current, nil
		}
		current = next
		seen[current.ID] = struct{}{}
		if strings.TrimSpace(current.ParentSessionID) == "" && (strings.TrimSpace(current.RootSessionID) == "" || strings.TrimSpace(current.RootSessionID) == current.ID) {
			return current, nil
		}
	}
	return current, nil
}

func normalizeProviderSafeMessages(msgs []message.Message) []message.Message {
	copied := deepCopyMessages(msgs)
	out := make([]message.Message, 0, len(copied))
	for i := 0; i < len(copied); i++ {
		msg := copied[i]
		if msg.Role == message.Tool {
			continue
		}
		if msg.Role != message.Assistant {
			out = append(out, msg)
			continue
		}

		msg.CleanIncompleteToolCalls()
		calls := msg.ToolCalls()
		if len(calls) == 0 {
			if len(msg.Parts) > 0 {
				out = append(out, msg)
			}
			continue
		}

		callIDs := make(map[string]struct{}, len(calls))
		for _, call := range calls {
			if call.ID != "" {
				callIDs[call.ID] = struct{}{}
			}
		}

		j := i + 1
		adjacentTools := make([]message.Message, 0)
		resultIDs := make(map[string]struct{}, len(callIDs))
		for j < len(copied) && copied[j].Role == message.Tool {
			toolMsg := copied[j]
			parts := make([]message.ContentPart, 0, len(toolMsg.Parts))
			for _, part := range toolMsg.Parts {
				tr, ok := part.(message.ToolResult)
				if !ok {
					continue
				}
				if _, wanted := callIDs[tr.ToolCallID]; !wanted {
					continue
				}
				resultIDs[tr.ToolCallID] = struct{}{}
				parts = append(parts, tr)
			}
			if len(parts) > 0 {
				toolMsg.Parts = parts
				adjacentTools = append(adjacentTools, toolMsg)
			}
			j++
		}

		parts := make([]message.ContentPart, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			tc, ok := part.(message.ToolCall)
			if !ok {
				parts = append(parts, part)
				continue
			}
			if _, hasResult := resultIDs[tc.ID]; hasResult {
				parts = append(parts, tc)
			}
		}
		if len(parts) > 0 {
			msg.Parts = parts
			out = append(out, msg)
			out = append(out, adjacentTools...)
		}
		i = j - 1
	}
	return out
}

func deepCopyMessages(msgs []message.Message) []message.Message {
	out := make([]message.Message, len(msgs))
	for i, msg := range msgs {
		out[i] = msg
		out[i].Parts = append([]message.ContentPart(nil), msg.Parts...)
	}
	return out
}

func (s *resumeService) fromDB(item db.Session) Session {
	return Session{
		ID:                  item.ID,
		ParentSessionID:     item.ParentSessionID.String,
		Title:               item.Title,
		MessageCount:        item.MessageCount,
		PromptTokens:        item.PromptTokens,
		CompletionTokens:    item.CompletionTokens,
		SummaryMessageID:    item.SummaryMessageID.String,
		Cost:                item.Cost,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
		Summary:             item.Summary.String,
		Tags:                item.Tags.String,
		FirstPrompt:         item.FirstPrompt.String,
		GitBranch:           item.GitBranch.String,
		ProjectPath:         item.ProjectPath,
		Worktree:            item.WorktreePath,
		Mode:                item.Mode,
		RootSessionID:       item.RootSessionID,
		ForkedFromSessionID: item.ForkedFromSessionID,
	}
}

func sanitizeResumeQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range q {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsSpace(r), r == '-', r == '_', r == '.', r == '/', r == '%':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func toFTSQuery(query string) string {
	tokens := strings.Fields(sanitizeResumeQuery(query))
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, tk := range tokens {
		if tk == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("\"%s\"*", strings.ReplaceAll(tk, "\"", "")))
	}
	return strings.Join(parts, " AND ")
}
