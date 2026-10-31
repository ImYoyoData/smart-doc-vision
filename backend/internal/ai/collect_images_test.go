package ai

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestCollectImagesInlineDataURL(t *testing.T) {
	input := model.TaskInput{Media: []model.MediaItem{
		{Type: "text", Content: "说明文本"},
		{Type: "image", URL: "https://example.com/a.png"},
		{Type: "image", Content: "data:image/png;base64,aGVsbG8=", Name: "inline.png"},
		{Type: "image", Content: "not-a-data-url"},
		{Type: "image", Key: "tasks/1/202610/a.jpg", Name: "stored.jpg"},
		{Type: "image"},
	}}
	images := CollectImages(input)
	if len(images) != 3 {
		t.Fatalf("want 3 images (url + inline data url + storage key), got %d", len(images))
	}
	if images[0].URL != "https://example.com/a.png" {
		t.Fatalf("url image lost: %+v", images[0])
	}
	if images[1].URL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("inline content not normalized to URL: %+v", images[1])
	}
	if images[1].Name != "inline.png" {
		t.Fatalf("inline image name lost: %+v", images[1])
	}
	if images[2].Key != "tasks/1/202610/a.jpg" {
		t.Fatalf("stored image key lost: %+v", images[2])
	}
}
