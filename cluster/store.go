package cluster

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	defaultKeyPrefix = "sentinel-sync-dns"
)

var (
	releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)
	renewLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)
)

type Store interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Exists(ctx context.Context, key string) (bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	CompareAndDelete(ctx context.Context, key, expectedValue string) (bool, error)
	CompareAndExpire(ctx context.Context, key, expectedValue string, ttl time.Duration) (bool, error)
}

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, value, ttl).Result()
}

func (s *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	count, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *RedisStore) CompareAndDelete(ctx context.Context, key, expectedValue string) (bool, error) {
	result, err := releaseLockScript.Run(ctx, s.client, []string{key}, expectedValue).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *RedisStore) CompareAndExpire(ctx context.Context, key, expectedValue string, ttl time.Duration) (bool, error) {
	result, err := renewLockScript.Run(ctx, s.client, []string{key}, expectedValue, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
