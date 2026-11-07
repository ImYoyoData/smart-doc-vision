package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"smartdoc-vision/backend/internal/httpx"
)

// publicCatalogURL is OpenRouter's public model catalog. It needs no API key
// and describes context window, max completion and input modalities for most
// open and vendor models, including the ones domestic aggregators re-export
// under their own IDs (for example deepseek-ai/DeepSeek-V4.1-Flash).
var publicCatalogURL = "https://openrouter.ai/api/v1/models"

const (
	publicCatalogTTL     = 6 * time.Hour
	publicCatalogTimeout = 6 * time.Second
)

type publicModelInfo struct {
	ContextWindow    int
	MaxOutputTokens  int
	SupportsVision   bool
	SupportsThinking bool
}

var publicCatalog = struct {
	mu      sync.RWMutex
	loading chan struct{}
	lastErr time.Time
	fetched time.Time
	models  map[string]publicModelInfo
}{}

// lookupPublicCatalog returns the catalog entry for a model ID. The catalog is
// fetched at most once per TTL and every failure is silent: it only fills gaps
// the provider and the built-in table could not answer.
func lookupPublicCatalog(ctx context.Context, modelID string) (publicModelInfo, bool) {
	key := normalizeCatalogKey(modelID)
	if key == "" {
		return publicModelInfo{}, false
	}
	models := loadPublicCatalog(ctx)
	info, ok := models[key]
	return info, ok
}

// loadPublicCatalog returns the cached catalog, refreshing it at most once per
// TTL. The network call runs without holding the lock, and concurrent callers
// wait for the single in-flight fetch instead of starting their own.
func loadPublicCatalog(ctx context.Context) map[string]publicModelInfo {
	publicCatalog.mu.RLock()
	models, fetched, lastErr := publicCatalog.models, publicCatalog.fetched, publicCatalog.lastErr
	inflight := publicCatalog.loading
	publicCatalog.mu.RUnlock()

	if models != nil && time.Since(fetched) < publicCatalogTTL {
		return models
	}
	// Back off for a minute after a failure so a blocked network does not make
	// every model-list request wait for the same timeout. lastErr is zero until
	// the first failure.
	if models != nil && !lastErr.IsZero() && time.Since(lastErr) < time.Minute {
		return models
	}
	if inflight != nil {
		select {
		case <-inflight:
		case <-ctx.Done():
			return models
		}
		publicCatalog.mu.RLock()
		defer publicCatalog.mu.RUnlock()
		return publicCatalog.models
	}

	publicCatalog.mu.Lock()
	if publicCatalog.loading != nil {
		done := publicCatalog.loading
		publicCatalog.mu.Unlock()
		<-done
		publicCatalog.mu.RLock()
		defer publicCatalog.mu.RUnlock()
		return publicCatalog.models
	}
	publicCatalog.loading = make(chan struct{})
	done := publicCatalog.loading
	publicCatalog.mu.Unlock()

	fresh, err := fetchPublicCatalog(ctx)

	publicCatalog.mu.Lock()
	if err != nil {
		// Keep serving the stale copy when a refresh fails; an empty map would
		// lose capabilities that were correct a moment ago.
		publicCatalog.lastErr = time.Now()
	} else {
		publicCatalog.models = fresh
		publicCatalog.fetched = time.Now()
	}
	publicCatalog.loading = nil
	result := publicCatalog.models
	publicCatalog.mu.Unlock()
	close(done)
	return result
}

// warmPublicCatalog starts the download in the background so the first
// "fetch models" click does not pay for it. Started once from the server.
func warmPublicCatalog() {
	go func() { loadPublicCatalog(context.Background()) }()
}

var catalogWarmOnce sync.Once

// WarmPublicCatalog is the exported entry point used at startup.
func WarmPublicCatalog() { catalogWarmOnce.Do(warmPublicCatalog) }

func fetchPublicCatalog(ctx context.Context) (map[string]publicModelInfo, error) {
	reqCtx, cancel := context.WithTimeout(ctx, publicCatalogTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, publicCatalogURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "smartdoc-vision-model-catalog")
	resp, err := httpx.Client(publicCatalogTimeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public catalog returned %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	models := make(map[string]publicModelInfo, len(payload.Data))
	for _, row := range payload.Data {
		id := firstStringValue(row, "id")
		key := normalizeCatalogKey(id)
		if key == "" {
			continue
		}
		info := publicModelInfo{
			ContextWindow: firstIntValue(row, "context_length", "context_window"),
		}
		if top, ok := row["top_provider"].(map[string]any); ok {
			if value := firstIntValue(top, "max_completion_tokens", "max_output_tokens"); value > 0 {
				info.MaxOutputTokens = value
			}
		}
		if architecture, ok := row["architecture"].(map[string]any); ok {
			if modalities, ok := architecture["input_modalities"].([]any); ok {
				for _, modality := range modalities {
					if strings.EqualFold(fmt.Sprint(modality), "image") {
						info.SupportsVision = true
					}
				}
			}
		}
		if params, ok := row["supported_parameters"].([]any); ok {
			for _, param := range params {
				switch strings.ToLower(fmt.Sprint(param)) {
				case "reasoning", "include_reasoning", "reasoning_effort", "thinking":
					info.SupportsThinking = true
				}
			}
		}
		models[key] = info
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("public catalog is empty")
	}
	return models, nil
}

// normalizeCatalogKey reduces a model ID to its comparable tail so that
// `deepseek-ai/DeepSeek-V4.1-Flash`, `deepseek/deepseek-v4.1-flash` and
// `DeepSeek-V4.1-Flash` all map to `deepseek-v4.1-flash`.
func normalizeCatalogKey(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	id = strings.TrimPrefix(id, "~")
	if index := strings.LastIndex(id, "/"); index >= 0 && index+1 < len(id) {
		id = id[index+1:]
	}
	id = strings.ReplaceAll(id, "_", "-")
	return strings.TrimSpace(id)
}

// enrichFromPublicCatalog fills any remaining unknown capability from the
// public catalog. It never overrides provider metadata or the built-in table.
func enrichFromPublicCatalog(ctx context.Context, item *providerModelItem) {
	if item.ContextWindowKnown && item.MaxOutputKnown && item.VisionKnown && item.ThinkingKnown {
		return
	}
	info, ok := lookupPublicCatalog(ctx, item.ID)
	if !ok {
		return
	}
	filled := false
	if !item.ContextWindowKnown && info.ContextWindow > 0 {
		item.ContextWindow = info.ContextWindow
		item.ContextWindowKnown = true
		filled = true
	}
	if !item.MaxOutputKnown && info.MaxOutputTokens > 0 {
		item.MaxOutputTokens = info.MaxOutputTokens
		item.MaxOutputKnown = true
		filled = true
	}
	if !item.VisionKnown {
		item.SupportsVision = info.SupportsVision
		item.VisionKnown = true
		filled = true
	}
	if !item.ThinkingKnown {
		item.SupportsThinking = info.SupportsThinking
		item.ThinkingKnown = true
		filled = true
	}
	if filled && item.Source != "metadata" {
		item.Source = "catalog"
	}
}
