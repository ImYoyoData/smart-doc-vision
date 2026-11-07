package catalog

import "strings"

// ModelCapability is a reference description of one model, used when the
// provider's `/models` endpoint returns no metadata at all (most domestic
// vendors only return bare IDs).
//
// Published limits change with vendor releases, so these values are starting
// points that the administrator can still edit in the form; the model picker
// labels them as coming from the built-in reference table rather than from the
// upstream API.
type ModelCapability struct {
	// Pattern is matched against the lower-case model ID using the rules below:
	// an entry with a version suffix wins over a shorter prefix entry.
	Pattern          string
	ContextWindow    int
	MaxOutputTokens  int
	SupportsVision   bool
	SupportsThinking bool
}

// modelCapabilityTable is ordered from most specific to most generic. Matching
// walks the list and returns the first entry whose pattern is a prefix of the
// model ID, so `deepseek-reasoner` must precede `deepseek`.
var modelCapabilityTable = []ModelCapability{
	// ---------- OpenAI ----------
	{Pattern: "gpt-5", ContextWindow: 400000, MaxOutputTokens: 128000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "gpt-4.1", ContextWindow: 1047576, MaxOutputTokens: 32768, SupportsVision: true, SupportsThinking: false},
	{Pattern: "gpt-4o-mini", ContextWindow: 128000, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: false},
	{Pattern: "gpt-4o", ContextWindow: 128000, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: false},
	{Pattern: "gpt-4-turbo", ContextWindow: 128000, MaxOutputTokens: 4096, SupportsVision: true, SupportsThinking: false},
	{Pattern: "o4-mini", ContextWindow: 200000, MaxOutputTokens: 100000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "o3", ContextWindow: 200000, MaxOutputTokens: 100000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "o1", ContextWindow: 200000, MaxOutputTokens: 100000, SupportsVision: true, SupportsThinking: true},

	// ---------- Anthropic ----------
	{Pattern: "claude-opus-4", ContextWindow: 200000, MaxOutputTokens: 32000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "claude-sonnet-4", ContextWindow: 200000, MaxOutputTokens: 64000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "claude-3-7-sonnet", ContextWindow: 200000, MaxOutputTokens: 64000, SupportsVision: true, SupportsThinking: true},
	{Pattern: "claude-3-5-sonnet", ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},
	{Pattern: "claude-3-5-haiku", ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},
	{Pattern: "claude", ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},

	// ---------- DeepSeek ----------
	{Pattern: "deepseek-reasoner", ContextWindow: 65536, MaxOutputTokens: 65536, SupportsVision: false, SupportsThinking: true},
	{Pattern: "deepseek-v3.2", ContextWindow: 163840, MaxOutputTokens: 65536, SupportsVision: false, SupportsThinking: true},
	{Pattern: "deepseek-v3.1", ContextWindow: 163840, MaxOutputTokens: 32768, SupportsVision: false, SupportsThinking: true},
	{Pattern: "deepseek-vl", ContextWindow: 32768, MaxOutputTokens: 4096, SupportsVision: true, SupportsThinking: false},
	{Pattern: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},

	// ---------- 通义千问 ----------
	{Pattern: "qwen3-vl", ContextWindow: 262144, MaxOutputTokens: 32768, SupportsVision: true, SupportsThinking: true},
	{Pattern: "qwen3-max", ContextWindow: 262144, MaxOutputTokens: 32768, SupportsVision: false, SupportsThinking: true},
	{Pattern: "qwen3-coder", ContextWindow: 262144, MaxOutputTokens: 65536, SupportsVision: false, SupportsThinking: false},
	{Pattern: "qwen3", ContextWindow: 131072, MaxOutputTokens: 16384, SupportsVision: false, SupportsThinking: true},
	{Pattern: "qwen2.5-vl", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},
	{Pattern: "qwen-max", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
	{Pattern: "qwen-plus", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
	{Pattern: "qwen-turbo", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
	{Pattern: "qwen-vl", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},
	{Pattern: "qwen", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},

	// ---------- 智谱 GLM ----------
	{Pattern: "glm-4.6v", ContextWindow: 131072, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: true},
	{Pattern: "glm-4.6", ContextWindow: 204800, MaxOutputTokens: 131072, SupportsVision: false, SupportsThinking: true},
	{Pattern: "glm-4.5v", ContextWindow: 65536, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: true},
	{Pattern: "glm-4.5", ContextWindow: 131072, MaxOutputTokens: 98304, SupportsVision: false, SupportsThinking: true},
	{Pattern: "glm-4", ContextWindow: 131072, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},

	// ---------- Kimi ----------
	{Pattern: "kimi-k2", ContextWindow: 262144, MaxOutputTokens: 98304, SupportsVision: false, SupportsThinking: true},
	{Pattern: "kimi-latest", ContextWindow: 262144, MaxOutputTokens: 98304, SupportsVision: true, SupportsThinking: false},
	{Pattern: "moonshot-v1-128k", ContextWindow: 131072, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},
	{Pattern: "moonshot-v1-32k", ContextWindow: 32768, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},
	{Pattern: "moonshot-v1-8k", ContextWindow: 8192, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},

	// ---------- 豆包 / 火山方舟 ----------
	{Pattern: "doubao-1.5-vision", ContextWindow: 131072, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: false},
	{Pattern: "doubao-seed-1.6-vision", ContextWindow: 256000, MaxOutputTokens: 32768, SupportsVision: true, SupportsThinking: true},
	{Pattern: "doubao-seed-1.6", ContextWindow: 256000, MaxOutputTokens: 32768, SupportsVision: true, SupportsThinking: true},
	{Pattern: "doubao-seed", ContextWindow: 256000, MaxOutputTokens: 16384, SupportsVision: true, SupportsThinking: true},
	{Pattern: "doubao", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},

	// ---------- 混元 ----------
	{Pattern: "hunyuan-t1", ContextWindow: 131072, MaxOutputTokens: 16384, SupportsVision: false, SupportsThinking: true},
	{Pattern: "hunyuan-vision", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: true, SupportsThinking: false},
	{Pattern: "hunyuan", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},

	// ---------- 文心千帆 ----------
	{Pattern: "ernie-4.5", ContextWindow: 131072, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: true},
	{Pattern: "ernie-4.0-8k", ContextWindow: 8192, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},
	{Pattern: "ernie", ContextWindow: 8192, MaxOutputTokens: 4096, SupportsVision: false, SupportsThinking: false},

	// ---------- MiniMax ----------
	{Pattern: "minimax-m", ContextWindow: 204800, MaxOutputTokens: 65536, SupportsVision: false, SupportsThinking: true},
	{Pattern: "abab", ContextWindow: 245760, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},

	// ---------- 小米 MiMo ----------
	{Pattern: "mimo-v2", ContextWindow: 1000000, MaxOutputTokens: 65536, SupportsVision: true, SupportsThinking: true},
	{Pattern: "mimo", ContextWindow: 262144, MaxOutputTokens: 16384, SupportsVision: false, SupportsThinking: true},

	// ---------- 讯飞星火 ----------
	{Pattern: "4.0ultra", ContextWindow: 32768, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
	{Pattern: "generalv3.5", ContextWindow: 32768, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
	{Pattern: "spark", ContextWindow: 32768, MaxOutputTokens: 8192, SupportsVision: false, SupportsThinking: false},
}

// LookupModelCapability returns the reference capability for a model ID. The
// model ID may carry a vendor prefix such as `deepseek/deepseek-chat` or
// `Qwen/Qwen3-...`; both the full ID and the last path segment are tried.
func LookupModelCapability(modelID string) (ModelCapability, bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ModelCapability{}, false
	}
	candidates := []string{id}
	if index := strings.LastIndex(id, "/"); index >= 0 && index+1 < len(id) {
		candidates = append(candidates, id[index+1:])
	}
	for _, candidate := range candidates {
		for _, entry := range modelCapabilityTable {
			if strings.HasPrefix(candidate, entry.Pattern) {
				return entry, true
			}
		}
	}
	return ModelCapability{}, false
}
