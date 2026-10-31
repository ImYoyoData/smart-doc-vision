package storage

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLocalStoreRoundTripAndSignedURL(t *testing.T) {
	store, err := NewLocal(Config{LocalDir: filepath.Join(t.TempDir(), "uploads"), SigningKey: "test-signing-key"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "tasks/1/202610/test.png"
	payload := []byte("image-bytes")
	if err := store.Put(ctx, key, payload, "image/png"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected payload: %q", got)
	}
	signed, err := store.URL(ctx, key, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	expires, err := strconv.ParseInt(parsed.Query().Get("e"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateSignedURL(key, expires, parsed.Query().Get("s")); err != nil {
		t.Fatalf("signed URL should validate: %v", err)
	}
	if err := store.ValidateSignedURL(key, expires, "bad-signature"); err == nil {
		t.Fatal("expected invalid signature to fail")
	}
	if err := store.ValidateSignedURL(key, time.Now().Add(-time.Minute).Unix(), parsed.Query().Get("s")); err == nil {
		t.Fatal("expected expired URL to fail")
	}
	if store.Root() == "" {
		t.Fatal("expected local store root")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key); err == nil {
		t.Fatal("expected missing object to fail")
	}
}

func TestNormalizeKeySafety(t *testing.T) {
	cases := map[string]string{
		"/tasks/1/a.png":   "tasks/1/a.png",
		"tasks//1/a.png":   "tasks/1/a.png",
		"tasks/../a.png":   "a.png",
		"tasks/1/中文.png":   "",
		"tasks/1/a b.png":  "",
		"":                 "",
		"..":               "",
		"tasks/1/a\\b.png": "tasks/1/a/b.png",
	}
	for input, want := range cases {
		got, err := NormalizeKey(input)
		if want == "" {
			if err == nil {
				t.Fatalf("expected %q to be rejected, got %q", input, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Fatalf("NormalizeKey(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestResolveS3Endpoint(t *testing.T) {
	endpoint, region, err := resolveS3Endpoint(Config{S3Provider: "aliyun", S3Region: "cn-shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://s3.oss-cn-shanghai.aliyuncs.com" || region != "cn-shanghai" {
		t.Fatalf("unexpected aliyun endpoint: %s %s", endpoint, region)
	}
	endpoint, _, err = resolveS3Endpoint(Config{S3Provider: "qiniu"})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://s3.cn-east-1.qiniucs.com" {
		t.Fatalf("unexpected qiniu endpoint: %s", endpoint)
	}
	if _, _, err := resolveS3Endpoint(Config{S3Provider: "custom"}); err == nil {
		t.Fatal("custom provider should require an endpoint")
	}
}
