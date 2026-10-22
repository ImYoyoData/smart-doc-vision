package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/service"
)

func (m *Manager) runRedisWorker(ctx context.Context, trigger model.TriggerSource) {
	var cfg RedisConfig
	_ = model.DecodeJSON(trigger.ConfigJSON, &cfg)
	if cfg.Stream == "" || cfg.Group == "" {
		return
	}
	if cfg.BatchSize <= 0 || cfg.BatchSize > 32 {
		cfg.BatchSize = 16
	}
	consumer := "sdv-worker-" + uuid.NewString()[:8]
	_ = m.redis.XGroupCreateMkStream(ctx, cfg.Stream, cfg.Group, "0").Err()
	m.touchRun(trigger.ID, "", false)
	lastClaim := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if time.Since(lastClaim) >= 30*time.Second {
			m.reclaimPending(ctx, trigger, cfg, consumer)
			lastClaim = time.Now()
		}
		streams, err := m.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: cfg.Group, Consumer: consumer,
			Streams: []string{cfg.Stream, ">"}, Count: int64(cfg.BatchSize), Block: 2 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			if strings.Contains(err.Error(), "NOGROUP") {
				_ = m.redis.XGroupCreateMkStream(ctx, cfg.Stream, cfg.Group, "0").Err()
				continue
			}
			m.markFailure(trigger.ID, err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				m.consumeRedisMessage(ctx, trigger, cfg, msg)
			}
		}
	}
}

func (m *Manager) reclaimPending(ctx context.Context, trigger model.TriggerSource, cfg RedisConfig, consumer string) {
	start := "0-0"
	for {
		msgs, next, err := m.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: cfg.Stream, Group: cfg.Group, Consumer: consumer,
			MinIdle: time.Minute, Start: start, Count: int64(cfg.BatchSize),
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("trigger %s reclaim pending: %v", cfg.Stream, err)
			}
			return
		}
		for _, msg := range msgs {
			m.consumeRedisMessage(ctx, trigger, cfg, msg)
		}
		if next == "" || next == "0-0" || len(msgs) == 0 {
			return
		}
		start = next
	}
}

func (m *Manager) consumeRedisMessage(ctx context.Context, trigger model.TriggerSource, cfg RedisConfig, msg redis.XMessage) {
	var payload string
	for _, key := range []string{"data", "payload"} {
		if v, ok := msg.Values[key]; ok {
			payload = fmt.Sprint(v)
			break
		}
	}
	if payload == "" && len(msg.Values) == 1 {
		for _, v := range msg.Values {
			payload = fmt.Sprint(v)
		}
	}
	var input model.TaskInput
	if err := json.Unmarshal([]byte(payload), &input); err != nil {
		m.deadLetter(ctx, cfg.Stream, msg, err)
		_ = m.redis.XAck(ctx, cfg.Stream, cfg.Group, msg.ID).Err()
		m.markFailure(trigger.ID, fmt.Errorf("消息 %s 解析失败: %w", msg.ID, err))
		return
	}
	if strings.TrimSpace(input.BizKey) == "" {
		input.BizKey = "redis:" + msg.ID
	}
	_, err := m.tasks.Submit(ctx, service.SubmitInput{
		ProjectID: trigger.ProjectID, AuditID: trigger.AuditID,
		SourceType: model.TriggerRedis, SourceID: trigger.ID, Input: input,
	})
	if err != nil {
		m.deadLetter(ctx, cfg.Stream, msg, err)
	}
	_ = m.redis.XAck(ctx, cfg.Stream, cfg.Group, msg.ID).Err()
	if err != nil {
		m.markFailure(trigger.ID, err)
	} else {
		m.touchRun(trigger.ID, "", true)
	}
}

func (m *Manager) deadLetter(ctx context.Context, stream string, msg redis.XMessage, err error) {
	payload := map[string]any{
		"source_id": msg.ID,
		"values":    msg.Values,
		"error":     truncate(err.Error(), 500),
		"failed_at": time.Now(),
	}
	b, _ := json.Marshal(payload)
	dlq := stream + ":dlq"
	if addErr := m.redis.XAdd(ctx, &redis.XAddArgs{Stream: dlq, Values: map[string]any{"data": string(b)}}).Err(); addErr != nil {
		log.Printf("trigger %s dlq write: %v", stream, addErr)
	}
}
