package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    int
	}{
		{"1.6.0", "1.7.0", -1},
		{"v1.6.0", "1.6.0", 0},
		{"1.6.0", "v1.5.9", 1},
		{"1.6", "1.6.0", 0},
		{"1.6.0-rc1", "1.6.0", 0},
		{"dev", "1.7.0", 0},
	}
	for _, item := range cases {
		if got := compareVersions(item.current, item.latest); got != item.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", item.current, item.latest, got, item.want)
		}
	}
}

func TestBuildUpdateCheckClassifiesAssets(t *testing.T) {
	published := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	release := githubRelease{
		TagName:     "v1.7.0",
		Name:        "SmartDoc Vision v1.7.0",
		HTMLURL:     "https://github.com/ImYoyoData/smart-doc-vision/releases/tag/v1.7.0",
		Body:        "- 新增功能",
		PublishedAt: published,
		Assets: []githubReleaseAsset{
			{Name: "smartdoc-vision-1.7.0-windows-amd64.exe", Size: 10, BrowserDownloadURL: "https://example.com/win"},
			{Name: "smartdoc-vision-1.7.0-windows-amd64-upx.exe", Size: 11, BrowserDownloadURL: "https://example.com/win-upx"},
			{Name: "smartdoc-vision-1.7.0-linux-amd64", Size: 12, BrowserDownloadURL: "https://example.com/linux"},
			{Name: "smartdoc-vision-1.7.0-linux-arm64", Size: 13, BrowserDownloadURL: "https://example.com/arm64"},
			{Name: "SHA256SUMS.txt", Size: 14, BrowserDownloadURL: "https://example.com/sums"},
		},
	}
	info := buildUpdateCheck("1.6.0", release, published)
	if info.CurrentVersion != "1.6.0" || info.LatestVersion != "1.7.0" || !info.HasUpdate {
		t.Fatalf("unexpected version result: %+v", info)
	}
	if len(info.Assets) != 5 || info.ChecksumsURL != "https://example.com/sums" {
		t.Fatalf("unexpected assets: %+v", info)
	}
	kinds := map[string]string{}
	for _, asset := range info.Assets {
		kinds[asset.Name] = asset.Kind
	}
	if kinds["smartdoc-vision-1.7.0-windows-amd64.exe"] != "windows-amd64" {
		t.Fatalf("windows asset was not classified: %+v", kinds)
	}
	if kinds["smartdoc-vision-1.7.0-linux-arm64"] != "linux-arm64" {
		t.Fatalf("arm64 asset was not classified: %+v", kinds)
	}
	if kinds["SHA256SUMS.txt"] != "checksum" {
		t.Fatalf("checksum asset was not classified: %+v", kinds)
	}
}

func TestBuildUpdateCheckHasNoUpdateForSameVersion(t *testing.T) {
	info := buildUpdateCheck("1.7.0", githubRelease{TagName: "v1.7.0"}, time.Now())
	if info.HasUpdate {
		t.Fatalf("same version must not be reported as an update: %+v", info)
	}
}

func TestFetchLatestRelease(t *testing.T) {
	var gotUserAgent, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tag_name":"v1.7.0","name":"Release","html_url":"https://example.com/release","published_at":"2026-10-30T10:00:00Z","assets":[]}`)
	}))
	defer server.Close()

	release, err := fetchLatestRelease(context.Background(), server.URL, server.Client())
	if err != nil {
		t.Fatalf("fetch release: %v", err)
	}
	if release.TagName != "v1.7.0" || release.HTMLURL != "https://example.com/release" {
		t.Fatalf("unexpected release: %+v", release)
	}
	if gotUserAgent == "" || gotAccept != "application/vnd.github+json" {
		t.Fatalf("unexpected request headers: user-agent=%q accept=%q", gotUserAgent, gotAccept)
	}
}

func TestFetchLatestReleaseRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()

	if _, err := fetchLatestRelease(context.Background(), server.URL, server.Client()); err == nil {
		t.Fatal("expected an error for a non-200 GitHub response")
	}
}
