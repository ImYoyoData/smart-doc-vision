package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/httpx"
	"smartdoc-vision/backend/internal/model"
	appversion "smartdoc-vision/backend/internal/version"
)

const (
	updateRepository = "ImYoyoData/smart-doc-vision"
	// The list endpoint is used instead of /releases/latest so the dialog can
	// show every release between the installed version and the newest one.
	updateAPIURL   = "https://api.github.com/repos/" + updateRepository + "/releases?per_page=30"
	updateRepoURL  = "https://github.com/" + updateRepository
	updateCacheKey = "sdv:update:releases"
	updateCacheTTL = 6 * time.Hour
	// updateReleaseKeep bounds how many releases are kept in the cache entry.
	updateReleaseKeep = 30
)

var updateHTTPClient = httpx.Client(8 * time.Second)

type githubRelease struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	HTMLURL     string               `json:"html_url"`
	Body        string               `json:"body"`
	PublishedAt time.Time            `json:"published_at"`
	Draft       bool                 `json:"draft"`
	Prerelease  bool                 `json:"prerelease"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type cachedUpdateReleases struct {
	Releases  []githubRelease `json:"releases"`
	FetchedAt time.Time       `json:"fetched_at"`
}

// updateReleaseNote is one release in the "what changed since your version"
// list. Notes keeps the raw Markdown body; the UI renders it inside the
// release's own card.
type updateReleaseNote struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
	Notes       string `json:"notes"`
	Prerelease  bool   `json:"prerelease"`
}

type updateAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

type updateCheck struct {
	CurrentVersion string        `json:"current_version"`
	LatestVersion  string        `json:"latest_version"`
	HasUpdate      bool          `json:"has_update"`
	ReleaseName    string        `json:"release_name"`
	ReleaseURL     string        `json:"release_url"`
	PublishedAt    string        `json:"published_at"`
	// Notes is the newest release body; kept for clients that only read one.
	Notes string `json:"notes"`
	// Releases lists every release newer than the running version, newest first.
	Releases     []updateReleaseNote `json:"releases"`
	// Assets are the download files of the newest release only.
	Assets       []updateAsset       `json:"assets"`
	ChecksumsURL string              `json:"checksums_url"`
	RepoURL      string              `json:"repo_url"`
	CheckedAt    string              `json:"checked_at"`
}

// systemUpdate checks the public GitHub release feed. It never downloads or
// replaces the running binary; the UI only shows release details and download
// guidance for the deployment mode.
func (s *Server) systemUpdate(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权检查更新")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	refresh := c.Query("refresh") == "1"
	cached, err := s.latestUpdateReleases(ctx, refresh)
	if err != nil {
		log.Printf("update check failed: %v", err)
		fail(c, http.StatusBadGateway, "UPDATE_CHECK_FAILED", "无法连接 GitHub 获取最新版本，请稍后重试", nil)
		return
	}
	ok(c, buildUpdateCheck(appversion.Version, cached.Releases, cached.FetchedAt))
}

func (s *Server) updateAutoCheck(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "system:config") {
		forbidden(c, "当前角色无权设置更新检查")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		badRequest(c, "enabled 必须是布尔值")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).
		Update("auto_update_check", *req.Enabled).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"enabled": *req.Enabled})
}

func (s *Server) latestUpdateReleases(ctx context.Context, refresh bool) (cachedUpdateReleases, error) {
	if !refresh {
		if cached, found := s.readUpdateCache(ctx); found {
			return cached, nil
		}
	}
	releases, err := fetchReleases(ctx, updateAPIURL, updateHTTPClient)
	if err != nil {
		return cachedUpdateReleases{}, err
	}
	cached := cachedUpdateReleases{Releases: releases, FetchedAt: time.Now().UTC()}
	s.writeUpdateCache(ctx, cached)
	return cached, nil
}

func (s *Server) readUpdateCache(ctx context.Context) (cachedUpdateReleases, bool) {
	if s.redis == nil {
		return cachedUpdateReleases{}, false
	}
	raw, err := s.redis.Get(ctx, updateCacheKey).Bytes()
	if err != nil {
		return cachedUpdateReleases{}, false
	}
	var cached cachedUpdateReleases
	if err := json.Unmarshal(raw, &cached); err != nil || len(cached.Releases) == 0 {
		return cachedUpdateReleases{}, false
	}
	if strings.TrimSpace(cached.Releases[0].TagName) == "" {
		return cachedUpdateReleases{}, false
	}
	return cached, true
}

func (s *Server) writeUpdateCache(ctx context.Context, cached cachedUpdateReleases) {
	if s.redis == nil {
		return
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		return
	}
	if err := s.redis.Set(ctx, updateCacheKey, raw, updateCacheTTL).Err(); err != nil {
		log.Printf("update cache write failed: %v", err)
	}
}

// fetchReleases reads the release list, skipping drafts and prereleases so the
// dialog never offers a build the maintainer has not published yet.
func fetchReleases(ctx context.Context, apiURL string, client *http.Client) ([]githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "smartdoc-vision-update-check")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github release api returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var listed []githubRelease
	if err := json.Unmarshal(body, &listed); err != nil {
		return nil, err
	}
	releases := make([]githubRelease, 0, len(listed))
	for _, release := range listed {
		if release.Draft || strings.TrimSpace(release.TagName) == "" {
			continue
		}
		if strings.TrimSpace(release.HTMLURL) == "" {
			release.HTMLURL = updateRepoURL + "/releases/tag/" + release.TagName
		}
		releases = append(releases, release)
		if len(releases) >= updateReleaseKeep {
			break
		}
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("github release list is empty")
	}
	return releases, nil
}

// buildUpdateCheck turns the release list into the dialog payload. Releases
// newer than the running version are returned newest-first so the dialog can
// show everything the operator is about to gain, not just the newest entry.
func buildUpdateCheck(current string, releases []githubRelease, fetchedAt time.Time) updateCheck {
	currentVersion := normalizeVersion(current)
	notes := make([]updateReleaseNote, 0, len(releases))
	for _, release := range releases {
		if compareVersions(current, normalizeVersion(release.TagName)) >= 0 {
			continue
		}
		notes = append(notes, updateReleaseNote{
			Version:     normalizeVersion(release.TagName),
			Name:        strings.TrimSpace(release.Name),
			URL:         release.HTMLURL,
			PublishedAt: formatUpdateTime(release.PublishedAt),
			Notes:       release.Body,
			Prerelease:  release.Prerelease,
		})
	}

	latestRelease := githubRelease{}
	if len(releases) > 0 {
		latestRelease = releases[0]
	}
	assets, checksumsURL := collectUpdateAssets(latestRelease.Assets)
	latest := normalizeVersion(latestRelease.TagName)
	return updateCheck{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		HasUpdate:      compareVersions(current, latest) < 0,
		ReleaseName:    strings.TrimSpace(latestRelease.Name),
		ReleaseURL:     latestRelease.HTMLURL,
		PublishedAt:    formatUpdateTime(latestRelease.PublishedAt),
		Notes:          latestRelease.Body,
		Releases:       notes,
		Assets:         assets,
		ChecksumsURL:   checksumsURL,
		RepoURL:        updateRepoURL,
		CheckedAt:      formatUpdateTime(fetchedAt),
	}
}

func collectUpdateAssets(list []githubReleaseAsset) ([]updateAsset, string) {
	assets := make([]updateAsset, 0, len(list))
	checksumsURL := ""
	for _, asset := range list {
		name := strings.TrimSpace(asset.Name)
		url := strings.TrimSpace(asset.BrowserDownloadURL)
		if name == "" || url == "" {
			continue
		}
		kind := classifyUpdateAsset(name)
		if kind == "checksum" {
			checksumsURL = url
		}
		assets = append(assets, updateAsset{
			Name: name,
			Size: asset.Size,
			URL:  url,
			Kind: kind,
		})
	}
	return assets, checksumsURL
}

func classifyUpdateAsset(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.EqualFold(name, "SHA256SUMS.txt"):
		return "checksum"
	case strings.Contains(lower, "windows") && strings.Contains(lower, "amd64"):
		return "windows-amd64"
	case strings.Contains(lower, "linux") && strings.Contains(lower, "arm64"):
		return "linux-arm64"
	case strings.Contains(lower, "linux") && strings.Contains(lower, "amd64"):
		return "linux-amd64"
	default:
		return "other"
	}
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	value = strings.TrimPrefix(value, "V")
	if index := strings.IndexAny(value, "-+"); index >= 0 {
		value = value[:index]
	}
	return value
}

func compareVersions(current, latest string) int {
	left, leftOK := parseVersionNumbers(current)
	right, rightOK := parseVersionNumbers(latest)
	if !leftOK || !rightOK {
		return 0
	}
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		var leftPart, rightPart int
		if index < len(left) {
			leftPart = left[index]
		}
		if index < len(right) {
			rightPart = right[index]
		}
		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
}

func parseVersionNumbers(value string) ([]int, bool) {
	value = normalizeVersion(value)
	if value == "" {
		return nil, false
	}
	parts := strings.Split(value, ".")
	numbers := make([]int, len(parts))
	for index, part := range parts {
		if part == "" {
			return nil, false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return nil, false
		}
		numbers[index] = number
	}
	return numbers, true
}

func formatUpdateTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
