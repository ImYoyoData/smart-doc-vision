package trigger

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestFingerprintTracksRoutingFields(t *testing.T) {
	connectionID := uint64(9)
	base := model.TriggerSource{
		ID: 3, Type: model.TriggerRedis, Status: "ACTIVE",
		RedisConnectionID: &connectionID, RedisDB: 2,
		Stream: "sdv:demo:audit:review", ConsumerGroup: "sdv-cg-3",
		StartPosition: "$", BatchSize: 16, MaxInflight: 1,
	}
	changed := base
	changed.BatchSize = 8
	changed.Stream = "sdv:demo:audit:other"
	if fingerprint(base) == fingerprint(changed) {
		t.Fatal("stream/batch changes must restart the worker")
	}
	otherDB := base
	otherDB.RedisDB = 3
	if fingerprint(base) == fingerprint(otherDB) {
		t.Fatal("DB changes must restart the worker")
	}
	if clientKey(9, 2) == clientKey(9, 3) {
		t.Fatal("clients must be isolated per Redis DB")
	}
}

func TestClientFingerprintChangesWithConnectionConfig(t *testing.T) {
	connection := model.RedisConnection{Addr: "127.0.0.1:6379", Username: "u"}
	first := clientFingerprint(connection, "p")
	connection.Addr = "127.0.0.1:6380"
	second := clientFingerprint(connection, "p")
	if first == second {
		t.Fatal("address changes must rebuild the Redis client")
	}
}
