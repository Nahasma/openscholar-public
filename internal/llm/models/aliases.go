package models

// ModelAliases maps user-friendly short names to canonical ModelIDs.
// All keys must be lowercase; ResolveModel lowercases input before lookup.
var ModelAliases = map[string]ModelID{
	// Anthropic
	"sonnet": Claude46Sonnet,
	"opus":   Claude47Opus,
	"haiku":  Claude45Haiku,
	"claude": Claude46Sonnet, // default Claude -> latest balanced Sonnet

	// OpenAI
	"gpt5":     GPT55,
	"gpt5mini": GPT54Mini,
	"gpt4":     GPT41,
	"gpt4mini": GPT41Mini,
	"o4mini":   O4Mini,

	// DeepSeek
	"deepseek":    DeepSeekChat,
	"r1":          DeepSeekR1,
	"deepseek-v4": DeepSeekV4Flash,
	"v4flash":     DeepSeekV4Flash,
	"v4pro":       DeepSeekV4Pro,

	// MiniMax
	"minimax": MiniMaxM27,

	// GLM
	"glm": GLM5,

	// SiliconFlow
	"sf-deepseek": SFDeepSeekV3,
	"sf-r1":       SFDeepSeekR1,
	"qwen":        SFQwen35397B,
	"qwen397b":    SFQwen35397B,
	"qwen122b":    SFQwen35122B,
	"qwen27b":     SFQwen3527B,
}
