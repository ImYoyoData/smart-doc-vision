package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LocalStore writes files below a single directory. Files are served by the
// backend through HMAC-signed /api/media URLs unless a public base URL is set.
type LocalStore struct {
	root       string
	publicBase string
	signingKey []byte
}

func NewLocal(cfg Config) (*LocalStore, error) {
	dir := strings.TrimSpace(cfg.LocalDir)
	if dir == "" {
		dir = filepath.Join("data", "uploads")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析本地存储目录失败: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("创建本地存储目录失败: %w", err)
	}
	return &LocalStore{
		root:       abs,
		publicBase: strings.TrimSpace(cfg.PublicBaseURL),
		signingKey: []byte(cfg.SigningKey),
	}, nil
}

func (s *LocalStore) Driver() string { return "local" }

func (s *LocalStore) Root() string { return s.root }

func (s *LocalStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建存储目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("设置文件权限失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}
	return nil
}

func (s *LocalStore) Get(ctx context.Context, key string) ([]byte, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("存储文件不存在")
		}
		return nil, fmt.Errorf("读取存储文件失败: %w", err)
	}
	return data, nil
}

func (s *LocalStore) Open(key string) (*os.File, os.FileInfo, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("存储文件不存在")
		}
		return nil, nil, fmt.Errorf("读取存储文件失败: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除存储文件失败: %w", err)
	}
	return nil
}

func (s *LocalStore) URL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	normalized, err := NormalizeKey(key)
	if err != nil {
		return "", err
	}
	if s.publicBase != "" {
		return joinURL(s.publicBase, normalized), nil
	}
	if len(s.signingKey) == 0 {
		return "", fmt.Errorf("本地存储未配置签名密钥")
	}
	expires := time.Now().Add(normalizeTTL(ttl)).Unix()
	return fmt.Sprintf("/api/media/%s?e=%d&s=%s", normalized, expires, s.sign(normalized, expires)), nil
}

func (s *LocalStore) ValidateSignedURL(key string, expires int64, signature string) error {
	normalized, err := NormalizeKey(key)
	if err != nil {
		return err
	}
	if time.Now().Unix() > expires {
		return fmt.Errorf("媒体地址已过期")
	}
	expected := s.sign(normalized, expires)
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return fmt.Errorf("媒体地址签名无效")
	}
	return nil
}

func (s *LocalStore) Test(ctx context.Context) error {
	key := "healthcheck/" + uuid.NewString() + ".txt"
	payload := []byte("smartdoc-vision storage probe")
	if err := s.Put(ctx, key, payload, "text/plain"); err != nil {
		return err
	}
	defer func() { _ = s.Delete(context.Background(), key) }()
	data, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	if string(data) != string(payload) {
		return fmt.Errorf("本地存储读写校验失败")
	}
	return nil
}

func (s *LocalStore) sign(key string, expires int64) string {
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(key + "\n" + strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *LocalStore) resolve(key string) (string, error) {
	normalized, err := NormalizeKey(key)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.root, filepath.FromSlash(normalized))
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("存储路径越界")
	}
	return path, nil
}
