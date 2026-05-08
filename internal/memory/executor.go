package memory

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/skillbank"
)

const executorPromptTemplate = `You are a memory management executor. Apply the selected skills to the input
text chunk and retrieved memories, then output memory actions.

Input Text Chunk:
%s

Retrieved Memories (0-based index):
%s

Selected Skills:
%s

Output format (repeat as needed):

ACTION: INSERT
MEMORY_ITEM: <concise summary>

ACTION: UPDATE
MEMORY_INDEX: <0-based index>
UPDATED_MEMORY: <merged summary>

ACTION: DELETE
MEMORY_INDEX: <0-based index>

ACTION: NOOP

If there is nothing worth remembering, output "ACTION: NOOP" only.`

// LLMCaller is a function type that makes a simple LLM call.
// Avoids importing the provider package (which would cause an import cycle).
type LLMCaller func(ctx context.Context, prompt string) (string, error)

// memoryService implements the Service interface.
type memoryService struct {
	store     *store
	skillBank skillbank.Service
	callLLM   LLMCaller
}

// NewService creates a new memory service.
// skillBank provides memory skills via List(ctx, "memory").
func NewService(q db.Querier, skillBank skillbank.Service, callLLM LLMCaller) Service {
	return &memoryService{
		store:     newStore(q),
		skillBank: skillBank,
		callLLM:   callLLM,
	}
}

func (s *memoryService) SetLLMCaller(caller LLMCaller) {
	s.callLLM = caller
}

func (s *memoryService) Insert(ctx context.Context, item MemoryItem) error {
	return s.store.Insert(ctx, item)
}

func (s *memoryService) Update(ctx context.Context, id string, content string, metadata map[string]any) error {
	return s.store.Update(ctx, id, content, metadata)
}

func (s *memoryService) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func (s *memoryService) RetrieveForSession(ctx context.Context, sessionID string, limit int) ([]MemoryItem, error) {
	return s.store.RetrieveForSession(ctx, sessionID, limit)
}

func (s *memoryService) RetrieveRecent(ctx context.Context, limit int) ([]MemoryItem, error) {
	return s.store.RetrieveRecent(ctx, limit)
}

func (s *memoryService) RetrieveForPrompt(ctx context.Context, sessionID string, opts PromptMemoryOptions) ([]MemoryItem, []MemoryItem, error) {
	if opts.SessionLimit <= 0 {
		opts.SessionLimit = DefaultPromptOptions.SessionLimit
	}
	if opts.GlobalLimit <= 0 {
		opts.GlobalLimit = DefaultPromptOptions.GlobalLimit
	}
	if opts.MaxItemChars <= 0 {
		opts.MaxItemChars = DefaultPromptOptions.MaxItemChars
	}

	var sessionItems, globalItems []MemoryItem

	if sessionID != "" {
		items, err := s.store.RetrieveSessionMemories(ctx, sessionID, opts.SessionLimit)
		if err != nil {
			return nil, nil, err
		}
		sessionItems = items
	}

	items, err := s.store.RetrieveGlobalMemories(ctx, opts.GlobalLimit)
	if err != nil {
		return nil, nil, err
	}
	globalItems = items

	// Truncate individual items
	truncate := func(items []MemoryItem) {
		for i := range items {
			if len(items[i].Content) > opts.MaxItemChars {
				items[i].Content = items[i].Content[:opts.MaxItemChars] + "..."
			}
		}
	}
	truncate(sessionItems)
	truncate(globalItems)

	return sessionItems, globalItems, nil
}

func (s *memoryService) CaptureKB(ctx context.Context, capture KBCapture) error {
	if strings.TrimSpace(capture.SessionID) == "" {
		// KB captures are session-scoped by default; skip global fallback.
		return nil
	}
	answer := strings.TrimSpace(capture.Answer)
	if answer == "" {
		return nil
	}
	return s.store.Insert(ctx, MemoryItem{
		SessionID: capture.SessionID,
		Content:   answer,
		Metadata:  buildKBMetadata(capture),
	})
}

// ExecuteSkills runs memory extraction on the given session text.
// Designed to be called asynchronously (in a goroutine).
func (s *memoryService) ExecuteSkills(ctx context.Context, sessionID string, sessionText string) error {
	if s.callLLM == nil {
		return nil
	}
	if s.skillBank == nil {
		return nil
	}

	skills, err := s.skillBank.List(ctx, "memory")
	if err != nil || len(skills) == 0 {
		return nil
	}

	memories, err := s.store.RetrieveForSession(ctx, sessionID, 10)
	if err != nil {
		return fmt.Errorf("retrieve memories: %w", err)
	}

	memoriesText := formatMemoriesForPrompt(memories)
	skillsText := formatSkillBankSkillsForPrompt(skills)
	prompt := fmt.Sprintf(executorPromptTemplate, sessionText, memoriesText, skillsText)

	output, err := s.callLLM(ctx, prompt)
	if err != nil {
		return fmt.Errorf("executor LLM call: %w", err)
	}

	actions := parseActions(output)

	for _, action := range actions {
		switch action.Type {
		case "INSERT":
			if action.Content != "" {
				_ = s.store.Insert(ctx, MemoryItem{
					SessionID: sessionID,
					Content:   action.Content,
					Metadata:  map[string]any{},
				})
			}
		case "UPDATE":
			if action.MemoryIndex >= 0 && action.MemoryIndex < len(memories) && action.Updated != "" {
				_ = s.store.Update(ctx, memories[action.MemoryIndex].ID, action.Updated, memories[action.MemoryIndex].Metadata)
			}
		case "DELETE":
			if action.MemoryIndex >= 0 && action.MemoryIndex < len(memories) {
				_ = s.store.Delete(ctx, memories[action.MemoryIndex].ID)
			}
		}
	}

	return nil
}

func buildKBMetadata(capture KBCapture) map[string]any {
	metadata := map[string]any{
		"source_type": capture.SourceType,
		"kind":        capture.Kind,
	}
	if capture.Question != "" {
		metadata["question"] = capture.Question
	}
	if capture.PaperID != "" {
		metadata["paper_id"] = capture.PaperID
	}
	if len(capture.Sources) > 0 {
		metadata["sources"] = capture.Sources
	}
	return metadata
}

func formatMemoriesForPrompt(memories []MemoryItem) string {
	if len(memories) == 0 {
		return "(no existing memories)"
	}
	var sb strings.Builder
	for i, m := range memories {
		fmt.Fprintf(&sb, "[%d] %s\n", i, m.Content)
	}
	return sb.String()
}

func formatSkillBankSkillsForPrompt(skills []skillbank.Skill) string {
	var sb strings.Builder
	for _, s := range skills {
		fmt.Fprintf(&sb, "### %s (%s)\n%s\n\n", s.Name, s.UpdateType, s.Instruction)
	}
	return sb.String()
}

var (
	actionRe      = regexp.MustCompile(`ACTION:\s*(INSERT|UPDATE|DELETE|NOOP)`)
	memoryItemRe  = regexp.MustCompile(`MEMORY_ITEM:\s*(.+)`)
	memoryIndexRe = regexp.MustCompile(`MEMORY_INDEX:\s*(\d+)`)
	updatedRe     = regexp.MustCompile(`UPDATED_MEMORY:\s*(.+)`)
)

func parseActions(output string) []MemoryAction {
	var actions []MemoryAction
	lines := strings.Split(output, "\n")

	var current *MemoryAction
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if match := actionRe.FindStringSubmatch(line); match != nil {
			if current != nil {
				actions = append(actions, *current)
			}
			current = &MemoryAction{Type: match[1], MemoryIndex: -1}
			continue
		}

		if current == nil {
			continue
		}

		if match := memoryItemRe.FindStringSubmatch(line); match != nil {
			current.Content = match[1]
		} else if match := memoryIndexRe.FindStringSubmatch(line); match != nil {
			idx, _ := strconv.Atoi(match[1])
			current.MemoryIndex = idx
		} else if match := updatedRe.FindStringSubmatch(line); match != nil {
			current.Updated = match[1]
		}
	}

	if current != nil {
		actions = append(actions, *current)
	}

	return actions
}
