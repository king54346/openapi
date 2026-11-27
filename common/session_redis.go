package common

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func sessionRedisKey(sessionID string) string {
	return RedisKey("session", sessionID)
}

func userSessionsRedisKey(userID int) string {
	return RedisKey("user_sessions", strconv.Itoa(userID))
}

// SaveRedisSession 将会话键值对持久化到 Redis，并维护用户活跃会话集合索引。
func SaveRedisSession(sessionID string, values map[string]any, ttl time.Duration) error {
	if !RedisEnabled || RDB == nil || sessionID == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	payload, err := Marshal(values)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pipe := RDB.Pipeline()
	pipe.Set(ctx, sessionRedisKey(sessionID), payload, ttl)
	if uid := extractSessionUserID(values["id"]); uid > 0 {
		uKey := userSessionsRedisKey(uid)
		pipe.SAdd(ctx, uKey, sessionID)
		pipe.Expire(ctx, uKey, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// LoadRedisSession 按 sessionID 从 Redis 读取会话数据；不存在或已过期时返回 (nil, false, nil)。
func LoadRedisSession(sessionID string) (map[string]any, bool, error) {
	if !RedisEnabled || RDB == nil || sessionID == "" {
		return nil, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	raw, err := RDB.Get(ctx, sessionRedisKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var values map[string]any
	if err := Unmarshal(raw, &values); err != nil {
		return nil, false, err
	}
	// JSON 数字反序列化默认是 float64，将已知整数字段还原为 int，与 cookie/gob 行为保持一致
	for _, k := range []string{"id", "role", "status"} {
		if f, ok := values[k].(float64); ok {
			values[k] = int(f)
		}
	}
	return values, true, nil
}

// DeleteRedisSession 销毁单个 Redis 会话并从所属用户集合中移除。
func DeleteRedisSession(sessionID string, userID int) error {
	if !RedisEnabled || RDB == nil || sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pipe := RDB.Pipeline()
	pipe.Del(ctx, sessionRedisKey(sessionID))
	if userID > 0 {
		pipe.SRem(ctx, userSessionsRedisKey(userID), sessionID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// RevokeUserSessions 强制注销指定用户的所有活跃 Redis 会话（禁用、删除、改密或升降权时调用）。
func RevokeUserSessions(userID int) {
	if !RedisEnabled || RDB == nil || userID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	uKey := userSessionsRedisKey(userID)
	sids, err := RDB.SMembers(ctx, uKey).Result()
	if err != nil {
		SysError("failed to list user sessions from redis: " + err.Error())
		return
	}
	keys := make([]string, 0, len(sids)+1)
	keys = append(keys, uKey)
	for _, sid := range sids {
		if sid != "" {
			keys = append(keys, sessionRedisKey(sid))
		}
	}
	if err := RDB.Del(ctx, keys...).Err(); err != nil {
		SysError("failed to revoke user sessions in redis: " + err.Error())
	}
}

func extractSessionUserID(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
