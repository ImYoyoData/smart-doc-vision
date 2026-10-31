package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

const maxDownloadBytes = 20 << 20

type Fetcher struct {
	AllowPrivate bool
	Client       *http.Client
}

type PreparedImage struct {
	Data   []byte
	Format string
	Width  int
	Height int
}

func NewFetcher(allowPrivate bool) *Fetcher {
	return &Fetcher{
		AllowPrivate: allowPrivate,
		Client: &http.Client{
			Timeout: 25 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("too many redirects")
				}
				if !allowPrivate {
					if err := validatePublicURL(req.URL); err != nil {
						return err
					}
				}
				return nil
			},
		},
	}
}

func (f *Fetcher) FetchImage(ctx context.Context, rawURL string) (*PreparedImage, error) {
	rawURL = strings.TrimSpace(rawURL)
	var data []byte
	if strings.HasPrefix(rawURL, "data:") {
		inline, err := decodeDataURL(rawURL)
		if err != nil {
			return nil, err
		}
		data = inline
	} else {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("图片地址无效: %w", err)
		}
		if !f.AllowPrivate {
			if err := validatePublicURL(u); err != nil {
				return nil, err
			}
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, errors.New("仅支持 http/https 图片地址")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "image/png,image/jpeg,image/*;q=0.8")
		req.Header.Set("User-Agent", "SmartDoc-Vision/1.0")
		resp, err := f.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("下载图片失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("下载图片失败: HTTP %d", resp.StatusCode)
		}
		limited := io.LimitReader(resp.Body, maxDownloadBytes+1)
		data, err = io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if len(data) > maxDownloadBytes {
			return nil, errors.New("图片超过 20MB 上限")
		}
	}
	return PrepareImage(data)
}

// PrepareImage decodes, validates and normalizes an image payload. It is shared
// by remote URL fetches and by files read back from the storage backend.
func PrepareImage(data []byte) (*PreparedImage, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片格式无效（仅支持 PNG/JPEG）: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return nil, fmt.Errorf("图片格式 %s 不受平台支持，请使用 PNG/JPEG", format)
	}
	resized, width, height, err := normalizeSize(img)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	switch format {
	case "jpeg":
		if err := jpeg.Encode(&out, resized, &jpeg.Options{Quality: 88}); err != nil {
			return nil, err
		}
	case "png":
		if err := png.Encode(&out, resized); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("不支持的图片编码")
	}
	if out.Len() > 16<<20 {
		return nil, errors.New("处理后的图片超过平台 16MB 上限")
	}
	return &PreparedImage{Data: out.Bytes(), Format: format, Width: width, Height: height}, nil
}

// decodeDataURL accepts a base64-encoded image data URL produced by the
// browser-side local uploader, so local files never have to be hosted publicly.
func decodeDataURL(raw string) ([]byte, error) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil, errors.New("图片 data URL 格式无效")
	}
	header := strings.ToLower(raw[:comma])
	if !strings.Contains(header, ";base64") || !strings.HasPrefix(header, "data:image/") {
		return nil, errors.New("仅支持 base64 编码的 PNG/JPEG data URL")
	}
	encoded := raw[comma+1:]
	if len(encoded) > base64.StdEncoding.EncodedLen(maxDownloadBytes) {
		return nil, errors.New("图片超过 20MB 上限")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("图片 base64 解码失败: %w", err)
	}
	if len(data) > maxDownloadBytes {
		return nil, errors.New("图片超过 20MB 上限")
	}
	return data, nil
}

func (f *Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func ValidateOutboundURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("地址无效: %w", err)
	}
	return validatePublicURL(u)
}

func validatePublicURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("仅支持 http/https 地址")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("地址缺少主机名")
	}
	if strings.EqualFold(host, "localhost") {
		return errors.New("出于 SSRF 防护，禁止访问本机地址")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("无法解析主机: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("出于 SSRF 防护，禁止访问内网地址 %s", ip.String())
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 carrier-grade NAT
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

func normalizeSize(src image.Image) (image.Image, int, int, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0, errors.New("图片尺寸无效")
	}
	const minSide, maxSide = 280, 1568
	scale := 1.0
	longSide := max(w, h)
	shortSide := min(w, h)
	if longSide > maxSide {
		scale = float64(maxSide) / float64(longSide)
	}
	if float64(shortSide)*scale < minSide {
		scale = float64(minSide) / float64(shortSide)
	}
	targetW := int(float64(w) * scale)
	targetH := int(float64(h) * scale)
	if targetW < minSide {
		targetW = minSide
	}
	if targetH < minSide {
		targetH = minSide
	}
	if targetW > maxSide || targetH > maxSide {
		return nil, 0, 0, fmt.Errorf("图片宽高比过于极端（%dx%d），无法满足 JEV 280~1568px 要求", w, h)
	}
	if targetW == w && targetH == h {
		return src, targetW, targetH, nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst, targetW, targetH, nil
}
