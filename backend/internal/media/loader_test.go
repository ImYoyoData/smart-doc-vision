package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/storage"
)

func TestLoaderPersistInline(t *testing.T) {
	store, err := storage.NewLocal(storage.Config{LocalDir: t.TempDir(), SigningKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(nil, store)

	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for x := 0; x < 320; x++ {
		for y := 0; y < 240; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	updated, persisted, err := loader.PersistInline(context.Background(), model.MediaItem{
		Type: "image", URL: dataURL, Name: "sample.png",
	}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted {
		t.Fatal("expected inline image to be persisted")
	}
	if updated.Key == "" || updated.URL != "" || updated.Content != "" {
		t.Fatalf("unexpected media item: %+v", updated)
	}
	if !strings.HasPrefix(updated.Key, "tasks/42/") || updated.Storage != "local" {
		t.Fatalf("unexpected storage metadata: %+v", updated)
	}
	prepared, err := loader.Fetch(context.Background(), updated)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Format != "png" || len(prepared.Data) == 0 {
		t.Fatalf("unexpected prepared image: %+v", prepared)
	}

	external := model.MediaItem{Type: "image", URL: "https://example.com/a.jpg"}
	if _, persisted, err := loader.PersistInline(context.Background(), external, 42); err != nil || persisted {
		t.Fatalf("external URL should not be persisted: %v %v", persisted, err)
	}
}
