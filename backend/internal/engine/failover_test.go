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

func TestOrderProviderCandidatesUsesPriorityAfterPrimary(t *testing.T) {
	providers := []model.AIProvider{
		{ID: 3, Priority: 10},
		{ID: 1, Priority: 50},
		{ID: 2, Priority: 20},
		{ID: 4, Priority: 5},
	}
	ordered := orderProviderCandidates(providers, 3, false)
	got := []uint64{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID}
	want := []uint64{3, 4, 2, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordered IDs = %v, want %v", got, want)
		}
	}
}
