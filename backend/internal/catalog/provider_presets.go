package catalog

import (
	"strings"

	"smartdoc-vision/backend/internal/model"
)

// ProviderPreset describes one well-known model vendor so the "add model"
// dialog can prefill the protocol, endpoint and default model instead of
// requiring an administrator to remember each OpenAI-compatible URL.
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

// ProviderPresets lists the vendors most deployments pick from. The list is
// ordered by how often domestic (CN) deployments choose them.
func ProviderPresets() []ProviderPreset {
	return []ProviderPreset{
		{
			Key: "deepseek", Name: "DeepSeek 深度求索", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat",
			SupportsVision: false, MaxImages: 0, MaxTokens: 8192, ContextWindow: 65536,
			Note:   "OpenAI 兼容；deepseek-chat 为通用对话，deepseek-reasoner 为推理模型。",
			DocURL: "https://api-docs.deepseek.com/", Tags: []string{"国内", "OpenAI 兼容"},
		},
		{
			Key: "volcengine", Name: "火山方舟（豆包）", Kind: model.ProviderOpenAI,
			BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Model: "doubao-seed-1-6-250615",
			SupportsVision: true, MaxImages: 6, MaxTokens: 16384, ContextWindow: 256000,
			Note:   "模型 ID 需填方舟控制台里的推理接入点 ID 或模型名；支持视觉与深度思考。",
			DocURL: "https://www.volcengine.com/docs/82379", Tags: []string{"国内", "视觉", "思考"},
		},
		{
			Key: "dashscope", Name: "阿里云百炼（通义千问）", Kind: model.ProviderOpenAI,
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus",
			SupportsVision: false, MaxImages: 0, MaxTokens: 8192, ContextWindow: 131072,
			Note:   "OpenAI 兼容模式；视觉模型选 qwen-vl 系列，思考模型选 qwen3 系列。",
			DocURL: "https://help.aliyun.com/zh/model-studio/", Tags: []string{"国内", "OpenAI 兼容"},
		},
		{
			Key: "zhipu", Name: "智谱 BigModel（GLM）", Kind: model.ProviderOpenAI,
			BaseURL: "https://open.bigmodel.cn/api/paas/v4", Model: "glm-4.6",
			SupportsVision: false, MaxImages: 0, MaxTokens: 98304, ContextWindow: 204800,
			Note:   "GLM-4.5V / 4.6V 支持图片；推理模型默认输出思考过程。",
			DocURL: "https://open.bigmodel.cn/dev/api", Tags: []string{"国内", "思考"},
		},
		{
			Key: "moonshot", Name: "月之暗面 Kimi", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.moonshot.cn/v1", Model: "kimi-k2-0905-preview",
			SupportsVision: false, MaxImages: 0, MaxTokens: 98304, ContextWindow: 262144,
			Note:   "Kimi K2 为长上下文通用模型；kimi-latest 会自动指向最新版本。",
			DocURL: "https://platform.moonshot.cn/docs", Tags: []string{"国内", "长上下文"},
		},
		{
			Key: "xiaomi", Name: "小米 MiMo", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.xiaomimimo.com/v1", Model: "mimo-v2-flash",
			SupportsVision: true, MaxImages: 6, MaxTokens: 65536, ContextWindow: 1000000,
			Note:   "小米开放平台的 OpenAI 兼容接口；模型 ID 以控制台「模型广场」为准。",
			DocURL: "https://platform.xiaomimimo.com/", Tags: []string{"国内", "视觉"},
		},
		{
			Key: "hunyuan", Name: "腾讯混元", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.hunyuan.cloud.tencent.com/v1", Model: "hunyuan-turbos-latest",
			SupportsVision: false, MaxImages: 0, MaxTokens: 16384, ContextWindow: 131072,
			Note:   "hunyuan-vision 支持图片；hunyuan-t1 为推理模型。",
			DocURL: "https://cloud.tencent.com/document/product/1729", Tags: []string{"国内"},
		},
		{
			Key: "qianfan", Name: "百度千帆（文心）", Kind: model.ProviderOpenAI,
			BaseURL: "https://qianfan.baidubce.com/v2", Model: "ernie-4.5-turbo-128k",
			SupportsVision: false, MaxImages: 0, MaxTokens: 8192, ContextWindow: 131072,
			Note:   "千帆 v2 使用 Bearer API Key；模型 ID 以千帆控制台为准。",
			DocURL: "https://cloud.baidu.com/doc/qianfan-api", Tags: []string{"国内"},
		},
		{
			Key: "minimax", Name: "MiniMax", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.minimaxi.com/v1", Model: "MiniMax-M2",
			SupportsVision: false, MaxImages: 0, MaxTokens: 65536, ContextWindow: 204800,
			Note:   "MiniMax 开放平台的 OpenAI 兼容接口。",
			DocURL: "https://platform.minimaxi.com/document", Tags: []string{"国内"},
		},
		{
			Key: "siliconflow", Name: "硅基流动 SiliconFlow", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.siliconflow.cn/v1", Model: "Qwen/Qwen3-235B-A22B-Instruct-2507",
			SupportsVision: false, MaxImages: 0, MaxTokens: 32768, ContextWindow: 262144,
			Note:   "聚合平台，同一 Key 可调用多家开源模型；模型 ID 含前级组织名。",
			DocURL: "https://docs.siliconflow.cn/", Tags: []string{"国内", "聚合"},
		},
		{
			Key: "spark", Name: "讯飞星火", Kind: model.ProviderOpenAI,
			BaseURL: "https://spark-api-open.xf-yun.com/v1", Model: "generalv3.5",
			SupportsVision: false, MaxImages: 0, MaxTokens: 8192, ContextWindow: 32768,
			Note:   "星火 HTTP 接口兼容 OpenAI 协议；图片理解需使用专门的视觉版本。",
			DocURL: "https://www.xfyun.cn/doc/spark/", Tags: []string{"国内"},
		},
		{
			Key: "openrouter", Name: "OpenRouter", Kind: model.ProviderOpenAI,
			BaseURL: "https://openrouter.ai/api/v1", Model: "openai/gpt-4o-mini",
			SupportsVision: true, MaxImages: 6, MaxTokens: 16384, ContextWindow: 128000,
			Note:   "聚合平台，`/models` 接口会返回完整的上下文、最大输出与模态信息。",
			DocURL: "https://openrouter.ai/docs", Tags: []string{"聚合", "元数据完整"},
		},
		{
			Key: "openai", Name: "OpenAI", Kind: model.ProviderOpenAI,
			BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini",
			SupportsVision: true, MaxImages: 6, MaxTokens: 16384, ContextWindow: 128000,
			Note:   "官方接口；gpt-4o / gpt-5 系列均支持图片输入。",
			DocURL: "https://platform.openai.com/docs", Tags: []string{"海外", "视觉"},
		},
		{
			Key: "anthropic", Name: "Anthropic Claude", Kind: model.ProviderAnthropic,
			BaseURL: "https://api.anthropic.com/v1", Model: "claude-sonnet-4-5",
			SupportsVision: true, MaxImages: 6, MaxTokens: 8192, ContextWindow: 200000,
			Note:   "使用 /messages 协议；图片会下载后以 base64 图片块提交。",
			DocURL: "https://docs.anthropic.com/", Tags: []string{"海外", "视觉", "思考"},
		},
		{
			Key: "ollama", Name: "Ollama 本地模型", Kind: model.ProviderOpenAI,
			BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen3:8b",
			SupportsVision: false, MaxImages: 0, MaxTokens: 4096, ContextWindow: 32768,
			Note:   "本机部署；若平台与 Ollama 不在同一台机器，需在 .env 里开启 MEDIA_ALLOW_PRIVATE。",
			DocURL: "https://ollama.com/", Tags: []string{"自托管"},
		},
	}
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
