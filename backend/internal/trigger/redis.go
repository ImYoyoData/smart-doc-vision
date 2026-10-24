package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/service"
)

func (m *Manager) runRedisWorker(ctx context.Context, trigger model.TriggerSource, client *redis.Client) {
	if strings.TrimSpace(trigger.Stream) == "" || strings.TrimSpace(trigger.ConsumerGroup) == "" {
		return
	}
	batchSize := trigger.BatchSize
	if batchSize <= 0 || batchSize > 32 {
		batchSize = 16
	}
	maxInflight := trigger.MaxInflight
	if maxInflight <= 0 {
		maxInflight = 1
	}
	if maxInflight > 32 {
		maxInflight = 32
	}
	startPosition := strings.TrimSpace(trigger.StartPosition)
	if startPosition != "0" {
		startPosition = "$"
	}
	consumer := "sdv-worker-" + uuid.NewString()[:8]
	_ = client.XGroupCreateMkStream(ctx, trigger.Stream, trigger.ConsumerGroup, startPosition).Err()
	m.touchRun(trigger.ID, "", false)
	lastClaim := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if time.Since(lastClaim) >= 30*time.Second {
			m.reclaimPending(ctx, trigger, client, batchSize, maxInflight)
			lastClaim = time.Now()
		}
		streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: trigger.ConsumerGroup, Consumer: consumer,
			Streams: []string{trigger.Stream, ">"}, Count: int64(batchSize), Block: 2 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			if strings.Contains(err.Error(), "NOGROUP") {
				_ = client.XGroupCreateMkStream(ctx, trigger.Stream, trigger.ConsumerGroup, startPosition).Err()
				continue
			}
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pingErr := client.Ping(pingCtx).Err()
			cancel()
			if pingErr != nil {
				m.connectionFailed(trigger, err)
				return
			}
			m.markFailure(trigger.ID, err)
			time.Sleep(2 * time.Second)
			continue
		}
		sem := make(chan struct{}, maxInflight)
		var wg sync.WaitGroup
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				sem <- struct{}{}
				wg.Add(1)
				go func(message redis.XMessage) {
					defer wg.Done()
					defer func() { <-sem }()
					m.consumeRedisMessage(ctx, trigger, client, message)
				}(msg)
			}
		}
		wg.Wait()
	}
}

func (m *Manager) reclaimPending(ctx context.Context, trigger model.TriggerSource, client *redis.Client, batchSize, maxInflight int) {
	start := "0-0"
	for {
		msgs, next, err := client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: trigger.Stream, Group: trigger.ConsumerGroup, Consumer: "sdv-reclaim-" + uuid.NewString()[:8],
			MinIdle: time.Minute, Start: start, Count: int64(batchSize),
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("trigger %s reclaim pending: %v", trigger.Stream, err)
			}
			return
		}
		sem := make(chan struct{}, maxInflight)
		var wg sync.WaitGroup
		for _, msg := range msgs {
			sem <- struct{}{}
			wg.Add(1)
			go func(message redis.XMessage) {
				defer wg.Done()
				defer func() { <-sem }()
				m.consumeRedisMessage(ctx, trigger, client, message)
			}(msg)
		}
		wg.Wait()
		if next == "" || next == "0-0" || len(msgs) == 0 {
			return
		}
		start = next
	}
}

func (m *Manager) consumeRedisMessage(ctx context.Context, trigger model.TriggerSource, client *redis.Client, msg redis.XMessage) {
	var payload string
	for _, key := range []string{"data", "payload"} {
		if value, ok := msg.Values[key]; ok {
			payload = fmt.Sprint(value)
			break
		}
	}
	if payload == "" && len(msg.Values) == 1 {
		for _, value := range msg.Values {
			payload = fmt.Sprint(value)
		}
	}
	var input model.TaskInput
	if err := json.Unmarshal([]byte(payload), &input); err != nil {
		m.deadLetter(ctx, client, trigger.Stream, msg, err)
		_ = client.XAck(ctx, trigger.Stream, trigger.ConsumerGroup, msg.ID).Err()
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
		m.deadLetter(ctx, client, trigger.Stream, msg, err)
	}
	_ = client.XAck(ctx, trigger.Stream, trigger.ConsumerGroup, msg.ID).Err()
	if err != nil {
		m.markFailure(trigger.ID, err)
	} else {
		m.touchRun(trigger.ID, "", true)
	}
}

func (m *Manager) connectionFailed(trigger model.TriggerSource, err error) {
	if trigger.RedisConnectionID == nil {
		m.markFailure(trigger.ID, err)
		return
	}
	var connection model.RedisConnection
	if findErr := m.db.First(&connection, *trigger.RedisConnectionID).Error; findErr == nil {
		m.markConnectionUnhealthy(connection, err)
		return
	}
	m.markFailure(trigger.ID, err)
}

func (m *Manager) deadLetter(ctx context.Context, client *redis.Client, stream string, msg redis.XMessage, err error) {
	payload := map[string]any{
		"source_id": msg.ID,
		"values":    msg.Values,
		"error":     truncate(err.Error(), 500),
		"failed_at": time.Now(),
	}
	data, _ := json.Marshal(payload)
	deadLetterStream := stream + ":dlq"
	if addErr := client.XAdd(ctx, &redis.XAddArgs{
		Stream: deadLetterStream, Values: map[string]any{"data": string(data)},
	}).Err(); addErr != nil {
		log.Printf("trigger %s dlq write: %v", stream, addErr)
	}
}
