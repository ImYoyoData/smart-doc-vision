package redisconn

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/model"
)

func New(connection model.RedisConnection, password string, db int) *redis.Client {
	options := &redis.Options{
		Addr:         connection.Addr,
		Username:     connection.Username,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if connection.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(options)
}

func Test(ctx context.Context, connection model.RedisConnection, password string) (time.Duration, error) {
	client := New(connection, password, connection.DB)
	defer client.Close()
	started := time.Now()
	err := client.Ping(ctx).Err()
	return time.Since(started), err
}
