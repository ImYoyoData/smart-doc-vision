package catalog

import (
	"strings"

	"smartdoc-vision/backend/internal/model"
)

// ProviderPreset describes one well-known model vendor so the "add model"
// dialog can prefill the protocol, endpoint and a mainstream default model
// instead of requiring an administrator to remember each OpenAI-compatible URL.
//
// Model IDs move fast: these are entry points, and the model list button next
// to the field is the authoritative source.
type ProviderPreset struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	SupportsVision bool     `json:"supports_vision"`
	MaxImages      int      `json:"max_images"`
	MaxTokens      int      `json:"max_tokens"`
	ContextWindow  int      `json:"context_window"`
	Note           string   `json:"note"`
	ModelsURL      string   `json:"models_url,omitempty"`
	DocURL         string   `json:"doc_url,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type presetSeed struct {
	key, name, kind, baseURL, model, note, docURL string
	maxImages                                     int
	tags                                          []string
	// vision/maxTokens/contextWindow override the reference table when the
	// vendor's default model is not covered by it.
	vision        bool
	thinking      bool
	maxTokens     int
	contextWindow int
}

// presetSeeds lists the vendors most deployments pick from, ordered by how
// often domestic (CN) deployments choose them. The default model is the
// vendor's current mainstream general-purpose model rather than the cheapest
// legacy one.
var presetSeeds = []presetSeed{
	{
		key: "deepseek", name: "DeepSeek 深度求索", kind: model.ProviderOpenAI,
		baseURL: "https://api.deepseek.com/v1", model: "deepseek-chat",
		maxImages: 0,
		note:      "OpenAI 兼容；deepseek-chat 为通用对话入口，deepseek-reasoner 为推理模型。具体模型的上下文与输出以「获取模型」带入的值为准（V4.1-Flash 为 100 万上下文并支持图片）。",
		docURL:    "https://api-docs.deepseek.com/",
		tags:      []string{"国内", "OpenAI 兼容"},
	},
	{
		key: "volcengine", name: "火山方舟（豆包）", kind: model.ProviderOpenAI,
		baseURL: "https://ark.cn-beijing.volces.com/api/v3", model: "doubao-seed-1-6-250615",
		maxImages: 6,
		note:      "模型 ID 需填方舟控制台里的推理接入点 ID 或模型名；豆包 Seed 系列支持视觉与深度思考。",
		docURL:    "https://www.volcengine.com/docs/82379",
		tags:      []string{"国内", "视觉", "思考"},
	},
	{
		key: "dashscope", name: "阿里云百炼（通义千问）", kind: model.ProviderOpenAI,
		baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", model: "qwen3.8-max",
		maxImages: 6,
		note:      "OpenAI 兼容模式；qwen3.8-max 为旗舰模型，视觉模型选 qwen3-vl 系列。模型 ID 以百炼控制台为准。",
		docURL:    "https://help.aliyun.com/zh/model-studio/",
		tags:      []string{"国内", "OpenAI 兼容", "视觉"},
	},
	{
		key: "zhipu", name: "智谱 BigModel（GLM）", kind: model.ProviderOpenAI,
		baseURL: "https://open.bigmodel.cn/api/paas/v4", model: "glm-5.3",
		maxImages: 6,
		note:      "glm-5.3 为旗舰通用模型；需要图片理解时选 glm-5v / glm-5.3-flashx 等视觉版本。",
		docURL:    "https://open.bigmodel.cn/dev/api",
		tags:      []string{"国内", "思考"},
	},
	{
		key: "moonshot", name: "月之暗面 Kimi", kind: model.ProviderOpenAI,
		baseURL: "https://api.moonshot.cn/v1", model: "kimi-k3",
		maxImages: 6,
		note:      "kimi-k3 为当前主力长上下文模型（100 万上下文、支持图片）；kimi-latest 会自动指向最新版本。",
		docURL:    "https://platform.moonshot.cn/docs",
		tags:      []string{"国内", "长上下文", "视觉"},
	},
	{
		key: "xiaomi", name: "小米 MiMo", kind: model.ProviderOpenAI,
		baseURL: "https://api.xiaomimimo.com/v1", model: "mimo-v2.6-flash",
		maxImages: 6,
		note:      "小米开放平台的 OpenAI 兼容接口；mimo-v2.6-flash 支持 100 万上下文与图片，模型 ID 以控制台「模型广场」为准。",
		docURL:    "https://platform.xiaomimimo.com/",
		tags:      []string{"国内", "视觉"},
	},
	{
		key: "hunyuan", name: "腾讯混元", kind: model.ProviderOpenAI,
		baseURL: "https://api.hunyuan.cloud.tencent.com/v1", model: "hy4",
		maxImages: 6,
		note:      "hy4 为混元第四代主力模型（100 万上下文、支持图片）；hunyuan-vision 为旧版视觉模型。",
		docURL:    "https://cloud.tencent.com/document/product/1729",
		tags:      []string{"国内", "视觉"},
	},
	{
		key: "qianfan", name: "百度千帆（文心）", kind: model.ProviderOpenAI,
		baseURL: "https://qianfan.baidubce.com/v2", model: "ernie-4.5-turbo-128k",
		maxImages: 0,
		note:      "千帆 v2 使用 Bearer API Key；模型 ID 以千帆控制台为准。",
		docURL:    "https://cloud.baidu.com/doc/qianfan-api",
		tags:      []string{"国内"},
	},
	{
		key: "minimax", name: "MiniMax", kind: model.ProviderOpenAI,
		baseURL: "https://api.minimaxi.com/v1", model: "MiniMax-M3",
		maxImages: 6,
		note:      "MiniMax M3 为旗舰模型（100 万上下文、支持图片）；OpenAI 兼容接口。",
		docURL:    "https://platform.minimaxi.com/document",
		tags:      []string{"国内", "视觉"},
	},
	{
		key: "siliconflow", name: "硅基流动 SiliconFlow", kind: model.ProviderOpenAI,
		baseURL: "https://api.siliconflow.cn/v1", model: "deepseek-ai/DeepSeek-V4.1-Flash",
		maxImages: 6,
		note:      "聚合平台，同一 Key 可调用多家开源模型；模型 ID 含前级组织名，例如 deepseek-ai/DeepSeek-V4.1-Flash。",
		docURL:    "https://docs.siliconflow.cn/",
		tags:      []string{"国内", "聚合"},
	},
	{
		key: "spark", name: "讯飞星火", kind: model.ProviderOpenAI,
		baseURL: "https://spark-api-open.xf-yun.com/v1", model: "generalv3.5",
		maxImages: 0,
		note:      "星火 HTTP 接口兼容 OpenAI 协议；图片理解需使用专门的视觉版本。",
		docURL:    "https://www.xfyun.cn/doc/spark/",
		tags:      []string{"国内"},
	},
	{
		key: "openrouter", name: "OpenRouter", kind: model.ProviderOpenAI,
		baseURL: "https://openrouter.ai/api/v1", model: "deepseek/deepseek-v4.1-flash",
		maxImages: 6,
		note:      "聚合平台，`/models` 接口会返回完整的上下文、最大输出与模态信息，能力以接口返回为准。",
		docURL:    "https://openrouter.ai/docs",
		tags:      []string{"聚合", "元数据完整"},
	},
	{
		key: "openai", name: "OpenAI", kind: model.ProviderOpenAI,
		baseURL: "https://api.openai.com/v1", model: "gpt-5.4",
		maxImages: 6,
		note:      "官方接口；gpt-5.4 为当前主力通用模型，支持图片与推理。",
		docURL:    "https://platform.openai.com/docs",
		tags:      []string{"海外", "视觉", "思考"},
	},
	{
		key: "anthropic", name: "Anthropic Claude", kind: model.ProviderAnthropic,
		baseURL: "https://api.anthropic.com/v1", model: "claude-sonnet-5",
		maxImages: 6,
		note:      "使用 /messages 协议；图片会下载后以 base64 图片块提交。",
		docURL:    "https://docs.anthropic.com/",
		tags:      []string{"海外", "视觉", "思考"},
	},
	{
		key: "ollama", name: "Ollama 本地模型", kind: model.ProviderOpenAI,
		baseURL: "http://127.0.0.1:11434/v1", model: "qwen3:8b",
		maxImages: 0,
		note:      "本机部署；若平台与 Ollama 不在同一台机器，需在 .env 里开启 MEDIA_ALLOW_PRIVATE。",
		docURL:    "https://ollama.com/",
		tags:      []string{"自托管"},
	},
}

// ProviderPresets fills each preset's capability numbers from the same
// reference table used by the model picker, so a preset and a freshly fetched
// model never disagree about context window or output limit.
func ProviderPresets() []ProviderPreset {
	presets := make([]ProviderPreset, 0, len(presetSeeds))
	for _, seed := range presetSeeds {
		preset := ProviderPreset{
			Key: seed.key, Name: seed.name, Kind: seed.kind,
			BaseURL: seed.baseURL, Model: seed.model, Note: seed.note,
			DocURL: seed.docURL, Tags: seed.tags,
			MaxImages: seed.maxImages, SupportsVision: seed.vision,
			ContextWindow: seed.contextWindow, MaxTokens: seed.maxTokens,
		}
		if reference, ok := LookupModelCapability(seed.model); ok {
			if preset.ContextWindow <= 0 {
				preset.ContextWindow = reference.ContextWindow
			}
			if preset.MaxTokens <= 0 {
				preset.MaxTokens = reference.MaxOutputTokens
			}
			preset.SupportsVision = preset.SupportsVision || reference.SupportsVision
		}
		if preset.SupportsVision && preset.MaxImages <= 0 {
			preset.MaxImages = 6
		}
		presets = append(presets, preset)
	}
	return presets
}

// FindProviderPreset returns the preset with the given key.
func FindProviderPreset(key string) (ProviderPreset, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, preset := range ProviderPresets() {
		if preset.Key == key {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}
