package api

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMain keeps the model-capability tests offline: an empty but fresh catalog
// makes lookups miss without hitting openrouter.ai.
func TestMain(m *testing.M) {
	publicCatalog.mu.Lock()
	publicCatalog.models = map[string]publicModelInfo{}
	publicCatalog.fetched = time.Now()
	publicCatalog.mu.Unlock()
	os.Exit(m.Run())
}

func TestEnrichProviderModelUsesReferenceCapabilities(t *testing.T) {
	// Domestic vendors usually return only bare IDs; the built-in reference table
	// must still produce usable context/output/vision/thinking values.
	item := parseProviderModel(map[string]any{"id": "deepseek-reasoner"})
	enrichProviderModel(context.Background(), &item)
	if !item.ContextWindowKnown || item.ContextWindow <= 0 {
		t.Fatalf("expected reference context window: %+v", item)
	}
	if !item.MaxOutputKnown || item.MaxOutputTokens <= 0 {
		t.Fatalf("expected reference max output: %+v", item)
	}
	if !item.ThinkingKnown || !item.SupportsThinking {
		t.Fatalf("deepseek-reasoner should be inferred as a thinking model: %+v", item)
	}
	if item.Source != "reference" {
		t.Fatalf("source should be reference, got %q", item.Source)
	}
}

func TestEnrichProviderModelHandlesVendorPrefixedID(t *testing.T) {
	// SiliconFlow returns IDs such as deepseek-ai/DeepSeek-V4.1-Flash. The model is
	// a 1M-context vision model, so the picker must not report 66K / text-only.
	item := parseProviderModel(map[string]any{"id": "deepseek-ai/DeepSeek-V4.1-Flash"})
	enrichProviderModel(context.Background(), &item)
	if !item.ContextWindowKnown || item.ContextWindow < 1000000 {
		t.Fatalf("expected ~1M context window, got %+v", item)
	}
	if !item.VisionKnown || !item.SupportsVision {
		t.Fatalf("DeepSeek-V4.1-Flash accepts images, got %+v", item)
	}
	if !item.ThinkingKnown || !item.SupportsThinking {
		t.Fatalf("DeepSeek-V4.1-Flash supports thinking, got %+v", item)
	}
}

func TestParseProviderModelPrefersUpstreamMetadata(t *testing.T) {
	item := parseProviderModel(map[string]any{
		"id":                "deepseek-chat",
		"context_length":    float64(12345),
		"max_output_tokens": float64(678),
		"supports_vision":   false,
	})
	if item.ContextWindow != 12345 || !item.ContextWindowKnown {
		t.Fatalf("upstream context window must win: %+v", item)
	}
	if item.MaxOutputTokens != 678 || !item.MaxOutputKnown {
		t.Fatalf("upstream max output must win: %+v", item)
	}
	if item.Source != "metadata" {
		t.Fatalf("source should stay metadata, got %q", item.Source)
	}
	// Enrichment must not overwrite numbers the vendor reported.
	enrichProviderModel(context.Background(), &item)
	if item.ContextWindow != 12345 || item.MaxOutputTokens != 678 || item.Source != "metadata" {
		t.Fatalf("enrichment overwrote provider metadata: %+v", item)
	}
}

func TestParseProviderModelUnknownKeepsSourceUnknown(t *testing.T) {
	item := parseProviderModel(map[string]any{"id": "some-private-model"})
	enrichProviderModel(context.Background(), &item)
	if item.Source != "unknown" {
		t.Fatalf("unmatched model should stay unknown, got %q", item.Source)
	}
	if item.ContextWindowKnown || item.MaxOutputKnown {
		t.Fatalf("unmatched model must not invent limits: %+v", item)
	}
}

func TestNormalizeCatalogKey(t *testing.T) {
	cases := map[string]string{
		"deepseek-ai/DeepSeek-V4.1-Flash": "deepseek-v4.1-flash",
		"deepseek/deepseek-v4.1-flash":    "deepseek-v4.1-flash",
		"Qwen/Qwen3-VL-30B":               "qwen3-vl-30b",
		"~deepseek/deepseek-v4-flash":     "deepseek-v4-flash",
		"bare-model":                      "bare-model",
		"":                                "",
	}
	for input, want := range cases {
		if got := normalizeCatalogKey(input); got != want {
			t.Fatalf("normalizeCatalogKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPublicCatalogParsing(t *testing.T) {
	// The catalog is written into a package-level cache; restore it afterwards so
	// other tests do not depend on this fake payload.
	publicCatalog.mu.Lock()
	originalModels, originalFetched := publicCatalog.models, publicCatalog.fetched
	publicCatalog.mu.Unlock()
	defer func() {
		publicCatalog.mu.Lock()
		publicCatalog.models, publicCatalog.fetched = originalModels, originalFetched
		publicCatalog.mu.Unlock()
	}()

	publicCatalog.mu.Lock()
	publicCatalog.models = map[string]publicModelInfo{
		"deepseek-v4.1-flash": {ContextWindow: 1048576, MaxOutputTokens: 943718, SupportsVision: true, SupportsThinking: true},
	}
	// A fresh timestamp keeps loadPublicCatalog from trying the network.
	publicCatalog.fetched = time.Now()
	publicCatalog.mu.Unlock()

	item := parseProviderModel(map[string]any{"id": "deepseek-ai/DeepSeek-V4.1-Flash"})
	enrichFromPublicCatalog(context.Background(), &item)
	if item.ContextWindow != 1048576 || !item.ContextWindowKnown {
		t.Fatalf("catalog context window not applied: %+v", item)
	}
	if item.Source != "catalog" {
		t.Fatalf("source should be catalog, got %q", item.Source)
	}
}
