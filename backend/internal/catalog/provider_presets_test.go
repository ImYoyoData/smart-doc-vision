package catalog

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestProviderPresetsAreUsable(t *testing.T) {
	presets := ProviderPresets()
	if len(presets) < 10 {
		t.Fatalf("expected a useful preset list, got %d entries", len(presets))
	}
	seen := map[string]bool{}
	for _, preset := range presets {
		if preset.Key == "" || preset.Name == "" {
			t.Fatalf("preset needs key and name: %+v", preset)
		}
		if seen[preset.Key] {
			t.Fatalf("duplicate preset key %q", preset.Key)
		}
		seen[preset.Key] = true
		if preset.Kind != model.ProviderOpenAI && preset.Kind != model.ProviderAnthropic {
			t.Fatalf("preset %s has unsupported kind %q", preset.Key, preset.Kind)
		}
		if preset.BaseURL == "" || preset.Model == "" {
			t.Fatalf("preset %s needs a base URL and a starting model", preset.Key)
		}
		if preset.MaxTokens < 0 || preset.ContextWindow < 0 {
			t.Fatalf("preset %s has negative limits: %+v", preset.Key, preset)
		}
		if preset.MaxTokens > preset.ContextWindow && preset.ContextWindow > 0 {
			t.Fatalf("preset %s sets max output above context window", preset.Key)
		}
	}
	for _, key := range []string{"deepseek", "volcengine", "xiaomi", "dashscope", "zhipu", "moonshot", "hunyuan", "qianfan", "siliconflow"} {
		if !seen[key] {
			t.Fatalf("missing expected domestic preset %q", key)
		}
	}
}

func TestFindProviderPreset(t *testing.T) {
	preset, ok := FindProviderPreset(" DeepSeek ")
	if !ok || preset.Key != "deepseek" {
		t.Fatalf("lookup failed: %+v ok=%v", preset, ok)
	}
	if _, ok := FindProviderPreset("nope"); ok {
		t.Fatal("unknown key must not resolve")
	}
}

func TestLookupModelCapabilityPrefersSpecificEntry(t *testing.T) {
	reasoner, ok := LookupModelCapability("deepseek-reasoner")
	if !ok || !reasoner.SupportsThinking {
		t.Fatalf("deepseek-reasoner should be a thinking model: %+v ok=%v", reasoner, ok)
	}
	chat, ok := LookupModelCapability("deepseek-chat")
	if !ok || chat.SupportsThinking {
		t.Fatalf("deepseek-chat should not be a thinking model: %+v ok=%v", chat, ok)
	}
}

func TestLookupModelCapabilityHandlesVendorPrefix(t *testing.T) {
	for _, id := range []string{"Qwen/Qwen3-235B-A22B-Instruct-2507", "qwen/qwen3-vl-30b-a3b-thinking", "openai/gpt-4o-mini"} {
		if _, ok := LookupModelCapability(id); !ok {
			t.Fatalf("prefixed model %q should resolve", id)
		}
	}
}

func TestLookupModelCapabilityUnknown(t *testing.T) {
	if _, ok := LookupModelCapability("totally-made-up-model"); ok {
		t.Fatal("unknown model must not resolve to a reference entry")
	}
	if _, ok := LookupModelCapability(""); ok {
		t.Fatal("empty id must not resolve")
	}
}
