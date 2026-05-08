package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Module is a composable prompt fragment with a name and priority.
type Module interface {
	// Name returns a unique identifier for this module.
	Name() string

	// Content returns the prompt text for this module.
	Content() string

	// Priority determines the order in the final prompt (lower = earlier).
	Priority() int
}

// CacheCategory marks a module's caching type for prompt cache optimization.
// 0 = static (cacheable), 1 = dynamic (not cacheable).
type CacheCategory = int

const (
	// CacheCategoryStatic indicates content that rarely changes and can be cached.
	CacheCategoryStatic CacheCategory = 0
	// CacheCategoryDynamic indicates content that changes frequently and should not be cached.
	CacheCategoryDynamic CacheCategory = 1
)

// CachedModule is an optional interface a Module can implement to declare its cache category.
// Modules that do not implement this interface are treated as CacheCategoryStatic.
// The return type is int (aliased as CacheCategory) to avoid circular imports with sub-packages.
type CachedModule interface {
	Module
	CacheCategory() int
}

// PromptBlock is a segment of the assembled prompt with cache metadata.
type PromptBlock struct {
	Text      string
	IsDynamic bool   // true → do not apply cache_control on this block
	CacheKey  string // stable hash for static blocks; empty for dynamic blocks
}

// PromptBuilder assembles multiple modules into a single system prompt.
type PromptBuilder struct {
	modules []Module
}

// NewPromptBuilder creates an empty builder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// Add appends a module to the builder. Supports chaining.
func (b *PromptBuilder) Add(m Module) *PromptBuilder {
	b.modules = append(b.modules, m)
	return b
}

// Build sorts modules by priority and concatenates their content.
// The original single-string method is preserved for backward compatibility with
// non-Anthropic providers.
func (b *PromptBuilder) Build() string {
	sort.SliceStable(b.modules, func(i, j int) bool {
		return b.modules[i].Priority() < b.modules[j].Priority()
	})

	var sb strings.Builder
	for i, m := range b.modules {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(m.Content())
	}
	return sb.String()
}

// BuildBlocks returns the assembled prompt split into cache-aware blocks.
// Adjacent modules with the same cache category are merged into a single block.
// Static blocks can have cache_control applied; dynamic blocks should not.
func (b *PromptBuilder) BuildBlocks() []PromptBlock {
	// Sort by priority (same ordering as Build)
	sort.SliceStable(b.modules, func(i, j int) bool {
		return b.modules[i].Priority() < b.modules[j].Priority()
	})

	var blocks []PromptBlock
	var currentText strings.Builder
	currentIsDynamic := false
	first := true

	for _, mod := range b.modules {
		content := mod.Content()
		if content == "" {
			continue
		}

		isDynamic := false
		if cm, ok := mod.(CachedModule); ok {
			isDynamic = cm.CacheCategory() == 1 // 1 == CacheCategoryDynamic
		}

		if !first && isDynamic != currentIsDynamic {
			// Category boundary — flush the current block
			if currentText.Len() > 0 {
				text := currentText.String()
				blocks = append(blocks, PromptBlock{
					Text:      text,
					IsDynamic: currentIsDynamic,
					CacheKey:  blockCacheKey(text, currentIsDynamic),
				})
				currentText.Reset()
			}
		}
		first = false
		currentIsDynamic = isDynamic

		if currentText.Len() > 0 {
			currentText.WriteString("\n\n")
		}
		currentText.WriteString(content)
	}

	// Flush the last block
	if currentText.Len() > 0 {
		text := currentText.String()
		blocks = append(blocks, PromptBlock{
			Text:      text,
			IsDynamic: currentIsDynamic,
			CacheKey:  blockCacheKey(text, currentIsDynamic),
		})
	}

	return blocks
}

// blockCacheKey computes a stable hash for static blocks.
// Dynamic blocks get an empty key (should not be cached).
func blockCacheKey(text string, isDynamic bool) string {
	if isDynamic {
		return ""
	}
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:8]) // 16-char hex, stable
}
