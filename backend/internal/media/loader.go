package media

import (
	"context"
	"fmt"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/storage"
)

// Loader resolves images either from external URLs, legacy inline data URLs,
// or object keys written by the storage backend.
type Loader struct {
	fetcher *Fetcher
	store   storage.Store
}

func NewLoader(fetcher *Fetcher, store storage.Store) *Loader {
	if fetcher == nil {
		fetcher = NewFetcher(false)
	}
	return &Loader{fetcher: fetcher, store: store}
}

func (l *Loader) Store() storage.Store { return l.store }

func (l *Loader) Fetch(ctx context.Context, item model.MediaItem) (*PreparedImage, error) {
	if key := strings.TrimSpace(item.Key); key != "" {
		if l.store == nil {
			return nil, fmt.Errorf("图片存储未配置")
		}
		data, err := l.store.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		return PrepareImage(data)
	}
	raw := strings.TrimSpace(item.URL)
	if raw == "" {
		raw = strings.TrimSpace(item.Content)
	}
	if raw == "" {
		return nil, fmt.Errorf("图片地址为空")
	}
	return l.fetcher.FetchImage(ctx, raw)
}

// PersistInline moves a browser-uploaded data URL into the storage backend and
// returns the rewritten media item. External http(s) URLs are left untouched.
func (l *Loader) PersistInline(ctx context.Context, item model.MediaItem, projectID uint64) (model.MediaItem, bool, error) {
	raw := inlineDataURL(item)
	if raw == "" {
		return item, false, nil
	}
	if l.store == nil {
		// Keep legacy inline behaviour when storage is unavailable (for example
		// in unit tests that construct a task service without a store).
		return item, false, nil
	}
	decoded, err := decodeDataURL(raw)
	if err != nil {
		return item, false, err
	}
	prepared, err := PrepareImage(decoded)
	if err != nil {
		return item, false, err
	}
	key := storage.BuildKey(projectID, prepared.Format, time.Now())
	contentType := storage.MIMEForFormat(prepared.Format)
	if err := l.store.Put(ctx, key, prepared.Data, contentType); err != nil {
		return item, false, err
	}
	item.URL = ""
	item.Content = ""
	item.Key = key
	item.Storage = l.store.Driver()
	item.MIME = contentType
	item.Size = int64(len(prepared.Data))
	return item, true, nil
}

func inlineDataURL(item model.MediaItem) string {
	if strings.HasPrefix(strings.TrimSpace(item.URL), "data:image/") {
		return strings.TrimSpace(item.URL)
	}
	if strings.HasPrefix(strings.TrimSpace(item.Content), "data:image/") {
		return strings.TrimSpace(item.Content)
	}
	return ""
}
