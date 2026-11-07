package catalog

import (
	"regexp"
	"strings"
)

// ModelCapability is a reference description of one model, used when the
// provider's `/models` endpoint returns no metadata at all (most domestic
// vendors only return bare IDs).
//
// Published limits change with vendor releases, so these values are starting
// points that the administrator can still edit in the form; the model picker
// labels them as coming from the built-in reference table rather than from the
// upstream API.
type ModelCapability struct {
	// ContextWindow is the total prompt + completion budget in tokens.
	ContextWindow int
	// MaxOutputTokens is the largest completion the model accepts.
	MaxOutputTokens  int
	SupportsVision   bool
	SupportsThinking bool
}

type capabilityRule struct {
	pattern  *regexp.Regexp
	capacity ModelCapability
}

func rule(pattern string, contextWindow, maxOutput int, vision, thinking bool) capabilityRule {
	return capabilityRule{
		pattern: regexp.MustCompile(pattern),
		capacity: ModelCapability{
			ContextWindow: contextWindow, MaxOutputTokens: maxOutput,
			SupportsVision: vision, SupportsThinking: thinking,
		},
	}
}

// modelCapabilityTable is ordered from most specific to most generic: matching
// returns the first rule that matches the normalised model ID, so
// `deepseek-v4` must precede `deepseek` and `glm-4.6v` must precede `glm-4.6`.
//
// Numbers follow the vendors' published specifications. Matching is anchored at
// the start of the ID, so a future `deepseek-v5` never silently inherits v4
// limits; it falls through to the family rule instead.
var modelCapabilityTable = []capabilityRule{
	// ---------- OpenAI ----------
	rule(`^gpt-6`, 1050000, 128000, true, true),
	rule(`^gpt-5\.[4-9]`, 1050000, 128000, true, true),
	rule(`^gpt-5`, 400000, 128000, true, true),
	rule(`^gpt-4\.1`, 1047576, 32768, true, false),
	rule(`^gpt-4o-mini`, 128000, 16384, true, false),
	rule(`^gpt-4o`, 128000, 16384, true, false),
	rule(`^gpt-4-turbo`, 128000, 4096, true, false),
	rule(`^o4`, 200000, 100000, true, true),
	rule(`^o3`, 200000, 100000, true, true),
	rule(`^o1`, 200000, 100000, true, true),

	// ---------- Anthropic ----------
	rule(`^claude-(?:opus|sonnet|haiku|fable)-5`, 1000000, 128000, true, true),
	rule(`^claude-(?:opus|sonnet)-4`, 200000, 64000, true, true),
	rule(`^claude-3-7-sonnet`, 200000, 64000, true, true),
	rule(`^claude`, 200000, 8192, true, false),

	// ---------- DeepSeek ----------
	// V4.x is a 1M-context family; the flash variants also accept images.
	rule(`^deepseek-v4\.1-flash`, 1048576, 943718, true, true),
	rule(`^deepseek-v4-flash-vision`, 1048576, 943717, true, true),
	rule(`^deepseek-v4-flash`, 1048576, 943717, false, true),
	rule(`^deepseek-v4-pro`, 1048576, 384000, false, true),
	rule(`^deepseek-v4`, 1048576, 131072, false, true),
	rule(`^deepseek-v3\.2`, 163840, 147456, false, true),
	rule(`^deepseek-v3\.1`, 163840, 147456, false, true),
	rule(`^deepseek-r2`, 163840, 65536, false, true),
	rule(`^deepseek-reasoner`, 163840, 32768, false, true),
	rule(`^deepseek-vl`, 32768, 4096, true, false),
	rule(`^deepseek`, 163840, 16384, false, false),

	// ---------- 通义千问 ----------
	rule(`^qwen3\.8-max`, 1000000, 131072, true, true),
	rule(`^qwen3\.8`, 1000000, 131072, true, true),
	rule(`^qwen3\.7-max`, 1000000, 131072, false, true),
	rule(`^qwen3\.7`, 1000000, 131072, true, true),
	rule(`^qwen3\.6`, 262144, 65536, true, true),
	rule(`^qwen3\.5`, 262144, 65536, true, true),
	rule(`^qwen3-vl`, 262144, 32768, true, true),
	rule(`^qwen3-max`, 262144, 32768, false, true),
	rule(`^qwen3-coder`, 262144, 65536, false, false),
	rule(`^qwen3`, 262144, 16384, false, true),
	rule(`^qwen2\.5-vl`, 131072, 8192, true, false),
	rule(`^qwen-max`, 131072, 8192, false, false),
	rule(`^qwen-plus`, 1000000, 32768, false, false),
	rule(`^qwen-turbo`, 131072, 8192, false, false),
	rule(`^qwen-vl`, 131072, 8192, true, false),
	rule(`^qwen`, 131072, 8192, false, false),

	// ---------- 智谱 GLM ----------
	rule(`^glm-5\.3-flashx`, 1048576, 131072, true, true),
	rule(`^glm-5\.3`, 1048576, 131072, false, true),
	rule(`^glm-5v`, 202752, 131072, true, true),
	rule(`^glm-5`, 204800, 131072, false, true),
	rule(`^glm-4\.7-flash`, 200000, 117964, false, true),
	rule(`^glm-4\.7`, 204800, 131072, false, true),
	rule(`^glm-4\.6v`, 131072, 32768, true, true),
	rule(`^glm-4\.6`, 204800, 131072, false, true),
	rule(`^glm-4\.5v`, 65536, 16384, true, true),
	rule(`^glm-4\.5`, 131072, 98304, false, true),
	rule(`^glm-4`, 131072, 4096, false, false),

	// ---------- Kimi ----------
	rule(`^kimi-k3\.1`, 1048576, 943718, true, true),
	rule(`^kimi-k3`, 1048576, 943718, true, true),
	rule(`^kimi-k2\.7-code`, 262144, 235929, true, true),
	rule(`^kimi-k2\.6`, 262144, 235929, true, true),
	rule(`^kimi-k2\.5`, 262144, 235929, true, true),
	rule(`^kimi-k2-thinking`, 262144, 98304, false, true),
	rule(`^kimi-k2`, 262144, 98304, false, false),
	rule(`^kimi-latest`, 1048576, 943718, true, false),
	rule(`^kimi`, 262144, 98304, false, false),
	rule(`^moonshot-v1-128k`, 131072, 4096, false, false),
	rule(`^moonshot-v1-32k`, 32768, 4096, false, false),
	rule(`^moonshot-v1-8k`, 8192, 4096, false, false),

	// ---------- 豆包 / 火山方舟 ----------
	rule(`^seed-2`, 262144, 131072, true, true),
	rule(`^seed-1\.6`, 262144, 32768, true, true),
	rule(`^doubao-seed-2`, 262144, 131072, true, true),
	rule(`^doubao-seed-1\.6-vision`, 262144, 32768, true, true),
	rule(`^doubao-seed-1\.6`, 262144, 32768, true, true),
	rule(`^doubao-seed`, 262144, 16384, true, true),
	rule(`^doubao-1\.5-vision`, 131072, 16384, true, false),
	rule(`^doubao`, 131072, 8192, false, false),

	// ---------- 腾讯混元 ----------
	rule(`^hy4`, 1048576, 65536, true, true),
	rule(`^hy3`, 262144, 128000, false, true),
	rule(`^hunyuan-t1`, 131072, 16384, false, true),
	rule(`^hunyuan-vision`, 131072, 8192, true, false),
	rule(`^hunyuan`, 131072, 8192, false, false),

	// ---------- 百度文心 ----------
	rule(`^ernie-4\.5`, 131072, 8192, false, true),
	rule(`^ernie`, 8192, 4096, false, false),

	// ---------- MiniMax ----------
	rule(`^minimax-m3\.1`, 1048576, 524288, true, true),
	rule(`^minimax-m3`, 1048576, 512000, true, true),
	rule(`^minimax-m2\.7`, 204800, 176947, false, true),
	rule(`^minimax-m2\.5`, 204800, 58982, false, true),
	rule(`^minimax-m1`, 1000000, 40000, false, true),
	rule(`^minimax-m`, 204800, 65536, false, true),
	rule(`^abab`, 245760, 8192, false, false),

	// ---------- 小米 MiMo ----------
	rule(`^mimo-v2\.6`, 1050000, 235929, true, true),
	rule(`^mimo-v2\.5`, 1050000, 235929, true, true),
	rule(`^mimo-v2`, 1000000, 65536, true, true),
	rule(`^mimo`, 262144, 16384, false, true),

	// ---------- 讯飞星火 ----------
	rule(`^4\.0ultra`, 32768, 8192, false, false),
	rule(`^generalv3\.5`, 32768, 8192, false, false),
	rule(`^spark`, 32768, 8192, false, false),

	// ---------- 阶跃星辰 ----------
	rule(`^step-3`, 262144, 230400, true, true),
	rule(`^step`, 262144, 65536, true, true),

	// ---------- 其他常见聚合模型 ----------
	rule(`^grok-4`, 500000, 450000, true, true),
	rule(`^gemini-3`, 1048576, 65536, true, true),
	rule(`^gemini`, 1048576, 65536, true, true),
	rule(`^mistral-large`, 1048576, 262144, true, true),
	rule(`^mistral`, 262144, 209715, true, true),
	rule(`^llama-4`, 524288, 32768, true, false),
	rule(`^llama-3`, 131072, 32768, false, false),
	rule(`^ling-3`, 1000000, 262144, false, true),
}

// LookupModelCapability returns the reference capability for a model ID. The
// model ID may carry a vendor prefix such as `deepseek-ai/DeepSeek-V4.1-Flash`
// or `Qwen/Qwen3-...`. The last path segment is tried first: the full ID also
// starts with the family name, so matching it first would let the generic
// `deepseek` rule win over the specific `deepseek-v4.1-flash` one.
func LookupModelCapability(modelID string) (ModelCapability, bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ModelCapability{}, false
	}
	candidates := []string{id}
	if index := strings.LastIndex(id, "/"); index >= 0 && index+1 < len(id) {
		candidates = []string{id[index+1:], id}
	}
	for _, candidate := range candidates {
		for _, entry := range modelCapabilityTable {
			if entry.pattern.MatchString(candidate) {
				return entry.capacity, true
			}
		}
	}
	return ModelCapability{}, false
}
