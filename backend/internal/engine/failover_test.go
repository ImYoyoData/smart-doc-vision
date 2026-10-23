package engine

import (
	"testing"
	"time"

	"smartdoc-vision/backend/internal/model"
)

func TestBreakerCooldownGrowsAndCaps(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{4, 240 * time.Second},
		{100, 10 * time.Minute},
	}
	for _, item := range cases {
		if got := breakerCooldown(item.failures); got != item.want {
			t.Fatalf("breakerCooldown(%d) = %s, want %s", item.failures, got, item.want)
		}
	}
}

func TestProviderCategoryKeepsJEVSeparate(t *testing.T) {
	if got := providerCategory(model.ProviderJEV); got != model.ProviderJEV {
		t.Fatalf("JEV category = %q", got)
	}
	for _, kind := range []string{model.ProviderOpenAI, model.ProviderAnthropic} {
		if got := providerCategory(kind); got != "GENERAL" {
			t.Fatalf("%s category = %q", kind, got)
		}
	}
}
