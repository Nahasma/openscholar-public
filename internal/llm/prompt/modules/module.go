package modules

// BaseModule is a reusable implementation of the Module interface.
// It is treated as a static (cacheable) module by default.
type BaseModule struct {
	name     string
	content  string
	priority int
}

// NewBaseModule creates a BaseModule with the given name, content, and priority.
func NewBaseModule(name, content string, priority int) BaseModule {
	return BaseModule{name: name, content: content, priority: priority}
}

func (m BaseModule) Name() string    { return m.name }
func (m BaseModule) Content() string { return m.content }
func (m BaseModule) Priority() int   { return m.priority }

// DynamicBaseModule wraps BaseModule and marks its content as dynamic (not cacheable).
// Use this for modules whose content changes frequently (e.g., memory, profile, conference).
type DynamicBaseModule struct {
	BaseModule
}

// NewDynamicBaseModule creates a DynamicBaseModule with the given name, content, and priority.
func NewDynamicBaseModule(name, content string, priority int) DynamicBaseModule {
	return DynamicBaseModule{BaseModule: NewBaseModule(name, content, priority)}
}

// CacheCategory returns 1 (CacheCategoryDynamic), indicating this module should not be cached.
// This satisfies the prompt.CachedModule interface without creating a circular import.
func (m DynamicBaseModule) CacheCategory() int { return 1 }
