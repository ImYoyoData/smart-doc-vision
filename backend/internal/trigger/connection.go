package trigger

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/redisconn"
)

func NewRedisClient(conn model.RedisConnection, password string, db int) *redis.Client {
	return redisconn.New(conn, password, db)
}

func TestRedisConnection(ctx context.Context, conn model.RedisConnection, password string) (time.Duration, error) {
	return redisconn.Test(ctx, conn, password)
}

// PauseTriggersForConnection pauses every active trigger bound to a connection.
// The pause reason distinguishes manual disabling from network failures.
func PauseTriggersForConnection(db *gorm.DB, connectionID uint64, reason string) error {
	return db.Model(&model.TriggerSource{}).
		Where("redis_connection_id = ? AND type = ? AND status = ?", connectionID, model.TriggerRedis, "ACTIVE").
		Updates(map[string]any{
			"status":       "PAUSED",
			"pause_reason": reason,
			"auto_paused":  true,
		}).Error
}

// ResumeTriggersForConnection only resumes triggers that the platform paused for
// the exact reason. Manually paused triggers are never touched.
func ResumeTriggersForConnection(db *gorm.DB, connectionID uint64, reason string) error {
	return db.Model(&model.TriggerSource{}).
		Where("redis_connection_id = ? AND type = ? AND status = ? AND pause_reason = ? AND auto_paused = ?",
			connectionID, model.TriggerRedis, "PAUSED", reason, true).
		Updates(map[string]any{
			"status":       "ACTIVE",
			"pause_reason": "",
			"auto_paused":  false,
		}).Error
}
