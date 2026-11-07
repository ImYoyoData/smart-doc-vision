package api

import "testing"

func TestParseProviderModelUsesReferenceCapabilities(t *testing.T) {
	// Domestic vendors usually return only bare IDs; the built-in reference table
	// must still produce usable context/output/vision/thinking values.
	item := parseProviderModel(map[string]any{"id": "deepseek-reasoner"})
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
}

func TestParseProviderModelUnknownKeepsSourceUnknown(t *testing.T) {
	item := parseProviderModel(map[string]any{"id": "some-private-model"})
	if item.Source != "unknown" {
		t.Fatalf("unmatched model should stay unknown, got %q", item.Source)
	}
	if item.ContextWindowKnown || item.MaxOutputKnown {
		t.Fatalf("unmatched model must not invent limits: %+v", item)
	}
}
