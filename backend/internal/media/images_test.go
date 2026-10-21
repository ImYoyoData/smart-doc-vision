package media

import (
	"image"
	"image/color"
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
