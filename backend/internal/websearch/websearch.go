package websearch

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/media"
)

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131 Safari/537.36"

var (
	tagPattern    = regexp.MustCompile(`(?is)<[^>]+>`)
	scriptPattern = regexp.MustCompile(`(?is)<(?:script|style|noscript)[^>]*>.*?</(?:script|style|noscript)>`)
	spacePattern  = regexp.MustCompile(`\s+`)
)

// Search uses Bing's RSS endpoint because it is stable, small, and does not
// require an API key. Results are deliberately capped to keep tool output
// small enough for the assistant context.
func Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if limit <= 0 || limit > 8 {
		limit = 5
	}
	endpoint := "https://www.bing.com/search?format=rss&q=" + url.QueryEscape(query)
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("搜索请求重定向次数过多")
			}
			host := strings.ToLower(req.URL.Hostname())
			if host != "bing.com" && !strings.HasSuffix(host, ".bing.com") {
				return errors.New("搜索请求被重定向到未知地址")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("联网搜索失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("搜索服务返回 HTTP %d", resp.StatusCode)
	}
	var feed struct {
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}
	results := make([]Result, 0, limit)
	seen := make(map[string]bool, limit)
	for _, item := range feed.Channel.Items {
		link := strings.TrimSpace(item.Link)
		title := cleanText(item.Title)
		if link == "" || title == "" || seen[link] {
			continue
		}
		seen[link] = true
		results = append(results, Result{
			Title: title, URL: link, Snippet: truncate(cleanText(item.Description), 320),
		})
		if len(results) >= limit {
			break
		}
	}
	if len(results) == 0 {
		return nil, errors.New("没有找到搜索结果")
	}
	return results, nil
}

// FetchText downloads a public web page and reduces it to plain text. The URL
// is validated before the request and on every redirect to prevent SSRF.
func FetchText(ctx context.Context, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if err := media.ValidateOutboundURL(rawURL); err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json;q=0.8,*/*;q=0.5")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("网页跳转次数过多")
			}
			return media.ValidateOutboundURL(next.URL.String())
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("读取网页失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("网页返回 HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "text/") && !strings.Contains(contentType, "json") && !strings.Contains(contentType, "xml") {
		return "", fmt.Errorf("网页类型 %s 不支持读取", contentType)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	text := cleanHTML(string(raw))
	text = truncate(text, 12000)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("网页没有可读取的文本内容")
	}
	return text, nil
}

func cleanHTML(value string) string {
	value = scriptPattern.ReplaceAllString(value, " ")
	value = tagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func cleanText(value string) string {
	value = html.UnescapeString(value)
	value = tagPattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
