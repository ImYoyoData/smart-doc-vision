package storage

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config describes the active file storage backend. Local storage keeps files
// under LocalDir; S3 storage works with any S3-compatible provider (AWS,
// Alibaba OSS, Tencent COS, Qiniu Kodo, MinIO and custom gateways).
type Config struct {
	Driver        string
	LocalDir      string
	PublicBaseURL string
	SigningKey    string

	S3Provider       string
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3Prefix         string
	S3ForcePathStyle bool
}

// Store is the storage abstraction used by uploads and the AI image loader.
type Store interface {
	Driver() string
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	URL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Test(ctx context.Context) error
}

func New(cfg Config) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "local", "fs", "filesystem":
		return NewLocal(cfg)
	case "s3", "oss", "cos", "qiniu", "kodo", "aliyun", "tencent", "aws", "minio", "object":
		return NewS3(cfg)
	default:
		return nil, fmt.Errorf("不支持的存储类型 %q", cfg.Driver)
	}
}

// NormalizeKey keeps object keys relative, slash-separated and inside the
// storage root. Only a conservative character set is accepted so the same key
// can be used for local paths and S3 object names.
func NormalizeKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return "", fmt.Errorf("存储路径为空")
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/':
		default:
			return "", fmt.Errorf("存储路径包含非法字符")
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+key), "/")
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("存储路径非法")
	}
	return clean, nil
}

// BuildKey returns a new immutable object key for an uploaded image.
func BuildKey(projectID uint64, format string, now time.Time) string {
	ext := ".jpg"
	if strings.EqualFold(format, "png") {
		ext = ".png"
	}
	return fmt.Sprintf("tasks/%d/%s/%s%s", projectID, now.Format("200601"), uuid.NewString(), ext)
}

func MIMEForFormat(format string) string {
	if strings.EqualFold(format, "png") {
		return "image/png"
	}
	return "image/jpeg"
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return time.Hour
	}
	if ttl > 7*24*time.Hour {
		return 7 * 24 * time.Hour
	}
	return ttl
}

func joinURL(base, key string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	key = strings.TrimLeft(key, "/")
	if base == "" {
		return "/" + key
	}
	return base + "/" + key
}
