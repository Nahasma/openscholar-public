package agent

import (
	"time"

	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/prompt"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
)

// CacheSafeSnapshot holds a point-in-time copy of the main Agent's cache-critical parameters.
// It is used by PostSamplingHook implementations that need to fork a sub-agent without
// sharing mutable state with the parent.
//
// Full implementation lives in forked.go (to be added later); this file contains only
// the type definition so that sampling_hooks.go can reference *CacheSafeSnapshot.
type CacheSafeSnapshot struct {
	SessionID     string
	Model         models.Model
	SystemMessage string
	SystemBlocks  []prompt.PromptBlock
	ActiveTools   []tools.BaseTool
	MessagePrefix []message.Message
	CapturedAt    time.Time
}
