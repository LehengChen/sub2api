package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var consumeOAuthSessionScript = redis.NewScript(`
local payload = redis.call("GET", KEYS[1])
if not payload then
  return nil
end
redis.call("DEL", KEYS[1])
return payload
`)

type redisOAuthSessionBackend struct {
	client redis.UniversalClient
}

var _ service.OAuthSessionBackend = (*redisOAuthSessionBackend)(nil)

func (b *redisOAuthSessionBackend) Set(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return b.client.Set(ctx, key, payload, ttl).Err()
}

func (b *redisOAuthSessionBackend) Get(ctx context.Context, key string) ([]byte, error) {
	payload, err := b.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrOAuthSessionBackendNotFound
	}
	return payload, err
}

func (b *redisOAuthSessionBackend) Consume(ctx context.Context, key string) ([]byte, error) {
	payload, err := consumeOAuthSessionScript.Run(ctx, b.client, []string{key}).Text()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrOAuthSessionBackendNotFound
	}
	return []byte(payload), err
}

func (b *redisOAuthSessionBackend) Delete(ctx context.Context, key string) error {
	return b.client.Del(ctx, key).Err()
}

// ProvideOAuthSessionStore keeps authorization state reachable from every API
// instance. Redis errors fail OAuth closed; multi-instance deployments must
// never silently fall back to process-local state.
func ProvideOAuthSessionStore(redisClient *redis.Client) (service.OAuthSessionStore, error) {
	if redisClient == nil {
		return nil, errors.New("redis oauth session client is required")
	}
	return service.NewRedisOAuthSessionStore(
		&redisOAuthSessionBackend{client: redisClient},
		service.DefaultOAuthSessionKeyPrefix,
	)
}

var acquireWorkerFenceScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  return {0, 0}
end
local token = redis.call("INCR", KEYS[2])
redis.call("SET", KEYS[1], ARGV[1] .. "|" .. token, "PX", ARGV[2])
return {1, token}
`)

var renewWorkerFenceScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
  return 1
end
return 0
`)

var releaseWorkerFenceScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type redisWorkerFenceBackend struct {
	client redis.UniversalClient
}

var _ service.WorkerFenceBackend = (*redisWorkerFenceBackend)(nil)

func (b *redisWorkerFenceBackend) Acquire(
	ctx context.Context,
	leaseKey, epochKey, owner string,
	ttl time.Duration,
) (int64, bool, error) {
	result, err := acquireWorkerFenceScript.Run(
		ctx,
		b.client,
		[]string{leaseKey, epochKey},
		owner,
		ttl.Milliseconds(),
	).Slice()
	if err != nil {
		return 0, false, err
	}
	if len(result) != 2 {
		return 0, false, fmt.Errorf("unexpected worker fence acquire result length %d", len(result))
	}

	acquired, err := redisIntegerResult(result[0])
	if err != nil {
		return 0, false, fmt.Errorf("decode worker fence acquire flag: %w", err)
	}
	switch acquired {
	case 0:
		return 0, false, nil
	case 1:
		token, err := redisIntegerResult(result[1])
		if err != nil {
			return 0, false, fmt.Errorf("decode worker fence token: %w", err)
		}
		return token, true, nil
	default:
		return 0, false, fmt.Errorf("unexpected worker fence acquire flag %d", acquired)
	}
}

func (b *redisWorkerFenceBackend) Renew(ctx context.Context, leaseKey, value string, ttl time.Duration) (bool, error) {
	result, err := renewWorkerFenceScript.Run(
		ctx,
		b.client,
		[]string{leaseKey},
		value,
		ttl.Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	if result != 0 && result != 1 {
		return false, fmt.Errorf("unexpected worker fence renew result %d", result)
	}
	return result == 1, nil
}

func (b *redisWorkerFenceBackend) Release(ctx context.Context, leaseKey, value string) error {
	result, err := releaseWorkerFenceScript.Run(ctx, b.client, []string{leaseKey}, value).Int()
	if err != nil {
		return err
	}
	if result != 0 && result != 1 {
		return fmt.Errorf("unexpected worker fence release result %d", result)
	}
	return nil
}

func redisIntegerResult(value any) (int64, error) {
	switch value := value.(type) {
	case int64:
		return value, nil
	case string:
		return strconv.ParseInt(value, 10, 64)
	case []byte:
		return strconv.ParseInt(string(value), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported Redis integer result type %T", value)
	}
}

func ProvideWorkerFence(redisClient *redis.Client, control runtimecontrol.Control) (*service.WorkerFence, error) {
	if redisClient == nil {
		return nil, errors.New("worker fence Redis client is required")
	}
	return service.NewWorkerFence(&redisWorkerFenceBackend{client: redisClient}, control)
}
