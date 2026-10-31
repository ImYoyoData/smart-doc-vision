package install

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/storage"
)

type ProbeResult struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	Note      string `json:"note,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

func TestAll(ctx context.Context, base config.Config, req Request) []ProbeResult {
	req.Normalize()
	results := make([]ProbeResult, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		results[0] = probeDatabase(ctx, req.Database)
	}()
	go func() {
		defer wg.Done()
		results[1] = probeRedis(ctx, req.Redis)
	}()
	go func() {
		defer wg.Done()
		results[2] = probeStorage(ctx, base, req)
	}()
	wg.Wait()
	return results
}

func AllOK(results []ProbeResult) bool {
	for _, result := range results {
		if !result.OK {
			return false
		}
	}
	return true
}

func probeDatabase(ctx context.Context, req Database) ProbeResult {
	started := time.Now()
	label := "MySQL"
	driver := "mysql"
	if req.Driver == config.DriverPostgres {
		label = "PostgreSQL"
		driver = "pgx"
	}
	result := ProbeResult{Key: "database", Label: label}
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()

	timeoutCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	db, err := sql.Open(driver, databaseDSN(req, true))
	if err != nil {
		result.Message = label + " 连接参数无效: " + err.Error()
		return result
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(timeoutCtx); err != nil {
		if unknownDatabaseError(err) {
			serverDB, openErr := sql.Open(driver, databaseDSN(req, false))
			if openErr == nil {
				defer serverDB.Close()
				serverDB.SetMaxOpenConns(1)
				if pingErr := serverDB.PingContext(timeoutCtx); pingErr == nil {
					result.OK = true
					result.Message = label + " 连接正常"
					result.Note = fmt.Sprintf("数据库 %s 尚不存在，安装时会自动创建", req.Database)
					return result
				}
			}
			result.Message = label + " 连接正常，但数据库不存在且无法自动创建"
			return result
		}
		result.Message = fmt.Sprintf("%s 连接失败: %s", label, databaseErrorMessage(err))
		return result
	}
	result.OK = true
	result.Message = label + " 连接正常"
	return result
}

func probeRedis(ctx context.Context, req Redis) ProbeResult {
	started := time.Now()
	result := ProbeResult{Key: "redis", Label: "Redis"}
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()

	client := redis.NewClient(&redis.Options{
		Addr:         net.JoinHostPort(req.Host, strconv.Itoa(req.Port)),
		Password:     req.Password,
		DB:           req.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer client.Close()
	timeoutCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := client.Ping(timeoutCtx).Err(); err != nil {
		result.Message = fmt.Sprintf("Redis 连接失败: %s", redisErrorMessage(err))
		return result
	}
	result.OK = true
	result.Message = fmt.Sprintf("Redis 连接正常（DB %d）", req.DB)
	return result
}

func probeStorage(ctx context.Context, base config.Config, req Request) ProbeResult {
	started := time.Now()
	result := ProbeResult{Key: "storage", Label: "文件存储"}
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()

	cfg := BuildConfig(base, req)
	store, err := storage.New(StorageConfig(cfg))
	if err != nil {
		result.Message = err.Error()
		return result
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := store.Test(timeoutCtx); err != nil {
		result.Message = err.Error()
		return result
	}
	result.OK = true
	if cfg.StorageDriver == "s3" {
		result.Message = "对象存储读写正常"
	} else {
		result.Message = "本地目录读写正常: " + cfg.StorageLocalDir
	}
	return result
}

func databaseErrorMessage(err error) string {
	message := err.Error()
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "access denied") || strings.Contains(lower, "password authentication failed"):
		return "用户名或密码错误，或该账号没有访问权限"
	case strings.Contains(lower, "connection refused"):
		return "无法连接，请检查地址、端口和网络/防火墙"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "连接超时，请检查网络"
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "name or service not known"):
		return "无法解析主机名"
	default:
		return message
	}
}

func redisErrorMessage(err error) string {
	message := err.Error()
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "wrongpass") || strings.Contains(lower, "invalid password") || strings.Contains(lower, "noauth"):
		return "Redis 密码错误或认证失败"
	case strings.Contains(lower, "connection refused"):
		return "无法连接，请检查地址、端口和网络/防火墙"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "连接超时，请检查网络"
	default:
		return message
	}
}
