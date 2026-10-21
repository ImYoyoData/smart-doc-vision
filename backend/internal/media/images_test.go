package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net"
	"testing"
)

func TestNormalizeSizeUpscalesSmallImages(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{R: 30, G: 100, B: 200, A: 255})
		}
	}
	out, w, h, err := normalizeSize(src)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || w != 560 || h != 280 {
		t.Fatalf("unexpected normalized size: w=%d h=%d", w, h)
	}
}

func TestNormalizeSizeDownscalesLargeImages(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3200, 1600))
	_, w, h, err := normalizeSize(src)
	if err != nil {
		t.Fatal(err)
	}
	if w != 1568 || h != 784 {
		t.Fatalf("unexpected normalized size: w=%d h=%d", w, h)
	}
}

func TestPrivateIPDetection(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.1.1", "100.64.0.1", "::1"} {
		if ip := net.ParseIP(raw); ip == nil || !isPrivateIP(ip) {
			t.Fatalf("expected %s to be blocked", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "203.0.113.10"} {
		if ip := net.ParseIP(raw); ip == nil || isPrivateIP(ip) {
			t.Fatalf("expected %s to be public", raw)
		}
	}
}

func TestFetchImageFromDataURL(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			src.Set(x, y, color.RGBA{R: 200, G: 220, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	prepared, err := NewFetcher(false).FetchImage(context.Background(), dataURL)
	if err != nil {
		t.Fatalf("fetch data URL: %v", err)
	}
	if prepared.Format != "png" || prepared.Width != 640 || prepared.Height != 480 {
		t.Fatalf("unexpected prepared image: %+v", prepared)
	}
}
