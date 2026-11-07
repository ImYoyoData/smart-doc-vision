package store

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/config"
)

const (
	// redisMaxIdleConns keeps a small warm pool; bursts open extra connections
	// on demand and they are closed again after ConnMaxIdleTime.
	redisMaxIdleConns = 8
	// redisMinPoolSize avoids tiny pools on single-core machines.
	redisMinPoolSize = 8
	// redisMaxPoolSize bounds memory used by socket buffers on big hosts.
	redisMaxPoolSize = 32
)

func redisPoolSize() int {
	size := runtime.GOMAXPROCS(0)
	if size < redisMinPoolSize {
		return redisMinPoolSize
	}
	if size > redisMaxPoolSize {
		return redisMaxPoolSize
	}
	return size
}

func OpenRedis(cfg config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		// go-redis defaults to 10 connections per CPU (160 on a 16-core host),
		// each with its own socket buffers. Queue de-duplication and locking
		// need far fewer, so cap the pool and let idle connections expire.
		PoolSize:        redisPoolSize(),
		MaxIdleConns:    redisMaxIdleConns,
		ConnMaxIdleTime: 5 * time.Minute,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis db %d: %w", cfg.RedisDB, err)
	}
	return client, nil
}
