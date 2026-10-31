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
	input, err := parseRedisInput(msg)
	input.SourceRef = msg.ID
	if err != nil {
		m.notifyRedisFailure(ctx, trigger, input, msg, "INVALID_MESSAGE", err)
		m.deadLetter(ctx, client, trigger.Stream, msg, err)
		_ = client.XAck(ctx, trigger.Stream, trigger.ConsumerGroup, msg.ID).Err()
		m.markFailure(trigger.ID, fmt.Errorf("消息 %s 解析失败: %w", msg.ID, err))
		return
	}
	if strings.TrimSpace(input.BizKey) == "" {
		input.BizKey = "redis:" + msg.ID
	}
	outcome, submitErr := m.tasks.Submit(ctx, service.SubmitInput{
		ProjectID: trigger.ProjectID, AuditID: trigger.AuditID,
		SourceType: model.TriggerRedis, SourceID: trigger.ID, Input: input,
	})
	if submitErr != nil {
		m.notifyRedisFailure(ctx, trigger, input, msg, triggerErrorCode(submitErr), submitErr)
		m.deadLetter(ctx, client, trigger.Stream, msg, submitErr)
	} else if outcome.Duplicated {
		_ = m.sendEvent(ctx, trigger.ProjectID, trigger.AuditID, input, "audit.deduplicated", map[string]any{
			"request_id": input.RequestID, "message_id": msg.ID, "source_ref": msg.ID,
			"source_type": model.TriggerRedis, "source_id": trigger.ID,
			"duplicate_of_task_no": outcome.Task.TaskNo, "task_no": outcome.Task.TaskNo,
			"status": outcome.Task.Status, "result": outcome.Task.Result,
			"biz_key": outcome.Task.BizKey, "duplicated": true,
			"received_at": time.Now().UTC().Format(time.RFC3339),
		})
	} else {
		_ = m.sendEvent(ctx, trigger.ProjectID, trigger.AuditID, input, "audit.accepted", map[string]any{
			"request_id": input.RequestID, "message_id": msg.ID, "source_ref": msg.ID,
			"source_type": model.TriggerRedis, "source_id": trigger.ID,
			"task_id": outcome.Task.ID, "task_no": outcome.Task.TaskNo,
			"status": outcome.Task.Status, "result": outcome.Task.Result,
			"biz_key":     outcome.Task.BizKey,
			"received_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
	_ = client.XAck(ctx, trigger.Stream, trigger.ConsumerGroup, msg.ID).Err()
	if submitErr != nil {
		m.markFailure(trigger.ID, submitErr)
	} else {
		m.touchRun(trigger.ID, "", true)
	}
}

func parseRedisInput(msg redis.XMessage) (model.TaskInput, error) {
	input := model.TaskInput{}
	if value, ok := msg.Values["request_id"]; ok {
		input.RequestID = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := msg.Values["callback_url"]; ok {
		input.CallbackURL = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := msg.Values["biz_key"]; ok {
		input.BizKey = strings.TrimSpace(fmt.Sprint(value))
	}
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
	if payload == "" {
		if input.RequestID == "" && input.BizKey == "" && input.CallbackURL == "" {
			return input, fmt.Errorf("消息缺少 data/payload")
		}
		return input, nil
	}
	var parsed model.TaskInput
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return input, err
	}
	if input.RequestID == "" {
		input.RequestID = parsed.RequestID
	}
	if input.CallbackURL == "" {
		input.CallbackURL = parsed.CallbackURL
	}
	if input.BizKey == "" {
		input.BizKey = parsed.BizKey
	}
	input.Media = parsed.Media
	input.Ext = parsed.Ext
	return input, nil
}

func triggerErrorCode(err error) string {
	if submitErr, ok := err.(*service.SubmitError); ok && submitErr.Code != "" {
		return submitErr.Code
	}
	return "SUBMIT_FAILED"
}

func (m *Manager) notifyRedisFailure(ctx context.Context, trigger model.TriggerSource, input model.TaskInput, msg redis.XMessage, code string, err error) {
	_ = m.sendEvent(ctx, trigger.ProjectID, trigger.AuditID, input, "audit.submit_failed", map[string]any{
		"request_id": input.RequestID, "message_id": msg.ID, "source_ref": msg.ID,
		"source_type": model.TriggerRedis, "source_id": trigger.ID,
		"stream": trigger.Stream, "biz_key": input.BizKey,
		"error_code": code, "error_message": truncate(err.Error(), 500),
		"failed_at": time.Now().UTC().Format(time.RFC3339),
	})
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
