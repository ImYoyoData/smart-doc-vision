package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxObjectBytes = 64 << 20

// S3Store talks to any S3-compatible endpoint. Alibaba OSS, Tencent COS and
// Qiniu Kodo expose S3-compatible gateways, so one implementation covers them
// alongside AWS S3 and MinIO.
type S3Store struct {
	client     *minio.Client
	bucket     string
	prefix     string
	publicBase string
}

func NewS3(cfg Config) (*S3Store, error) {
	endpoint, region, err := resolveS3Endpoint(cfg)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(cfg.S3Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("对象存储 Bucket 不能为空")
	}
	if strings.TrimSpace(cfg.S3AccessKey) == "" || strings.TrimSpace(cfg.S3SecretKey) == "" {
		return nil, fmt.Errorf("对象存储 AccessKey / SecretKey 不能为空")
	}
	secure := true
	host := endpoint
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") {
		secure = false
		host = endpoint[len("http://"):]
	} else if strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		host = endpoint[len("https://"):]
	}
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		return nil, fmt.Errorf("对象存储 Endpoint 不能为空")
	}
	lookup := minio.BucketLookupAuto
	if cfg.S3ForcePathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(strings.TrimSpace(cfg.S3AccessKey), strings.TrimSpace(cfg.S3SecretKey), ""),
		Secure:       secure,
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化对象存储客户端失败: %w", err)
	}
	return &S3Store{
		client:     client,
		bucket:     bucket,
		prefix:     strings.Trim(strings.TrimSpace(cfg.S3Prefix), "/"),
		publicBase: strings.TrimSpace(cfg.PublicBaseURL),
	}, nil
}

func (s *S3Store) Driver() string { return "s3" }

func (s *S3Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	object, err := s.objectKey(key)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = s.client.PutObject(ctx, s.bucket, object, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("上传对象存储失败: %s", s3ErrorMessage(err))
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.objectKey(key)
	if err != nil {
		return nil, err
	}
	reader, err := s.client.GetObject(ctx, s.bucket, object, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取对象存储失败: %s", s3ErrorMessage(err))
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxObjectBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取对象存储失败: %s", s3ErrorMessage(err))
	}
	if len(data) > maxObjectBytes {
		return nil, fmt.Errorf("对象存储文件超过 64MB 上限")
	}
	return data, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	object, err := s.objectKey(key)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, object, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除对象存储文件失败: %s", s3ErrorMessage(err))
	}
	return nil
}

func (s *S3Store) URL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	object, err := s.objectKey(key)
	if err != nil {
		return "", err
	}
	if s.publicBase != "" {
		return joinURL(s.publicBase, object), nil
	}
	value, err := s.client.PresignedGetObject(ctx, s.bucket, object, normalizeTTL(ttl), url.Values{})
	if err != nil {
		return "", fmt.Errorf("生成对象存储访问地址失败: %s", s3ErrorMessage(err))
	}
	return value.String(), nil
}

func (s *S3Store) Test(ctx context.Context) error {
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
	if !bytes.Equal(data, payload) {
		return fmt.Errorf("对象存储读写校验失败")
	}
	return nil
}

func (s *S3Store) objectKey(key string) (string, error) {
	normalized, err := NormalizeKey(key)
	if err != nil {
		return "", err
	}
	if s.prefix == "" {
		return normalized, nil
	}
	return s.prefix + "/" + normalized, nil
}

func resolveS3Endpoint(cfg Config) (string, string, error) {
	endpoint := strings.TrimSpace(cfg.S3Endpoint)
	region := strings.TrimSpace(cfg.S3Region)
	provider := strings.ToLower(strings.TrimSpace(cfg.S3Provider))
	if provider == "" {
		provider = "custom"
	}
	defaultRegion := func(value string) string {
		if region != "" {
			return region
		}
		return value
	}
	if endpoint == "" {
		switch provider {
		case "aliyun", "ali", "oss":
			region = defaultRegion("cn-hangzhou")
			endpoint = "https://s3.oss-" + region + ".aliyuncs.com"
		case "tencent", "cos":
			region = defaultRegion("ap-guangzhou")
			endpoint = "https://cos." + region + ".myqcloud.com"
		case "qiniu", "kodo":
			region = defaultRegion("cn-east-1")
			endpoint = "https://s3." + region + ".qiniucs.com"
		case "aws", "amazon":
			region = defaultRegion("us-east-1")
			endpoint = "https://s3." + region + ".amazonaws.com"
		default:
			return "", region, fmt.Errorf("对象存储 Endpoint 不能为空（MinIO/自定义 S3 需要填写完整地址）")
		}
	}
	if !strings.HasPrefix(strings.ToLower(endpoint), "http://") && !strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		lower := strings.ToLower(endpoint)
		if strings.HasPrefix(lower, "localhost") || strings.HasPrefix(lower, "127.0.0.1") {
			endpoint = "http://" + endpoint
		} else {
			endpoint = "https://" + endpoint
		}
	}
	return endpoint, region, nil
}

func s3ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchBucket" {
		return "Bucket 不存在"
	}
	if response.Code == "AccessDenied" || response.Code == "InvalidAccessKeyId" || response.Code == "SignatureDoesNotMatch" {
		return "AccessKey / SecretKey 无效或没有该 Bucket 的权限"
	}
	if response.Message != "" {
		return response.Message
	}
	return err.Error()
}
