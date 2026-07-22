//go:build unit

package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func activeFenceControl(instance string) runtimecontrol.Control {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleActive
	control.InstanceID = instance
	control.WorkerLeaseKey = "sub2api:test:worker-fence"
	control.WorkerLeaseTTL = 300 * time.Millisecond
	control.WorkerRenewInterval = 50 * time.Millisecond
	return control
}

func TestWorkerFenceAllowsExactlyOneActiveAndUsesMonotonicTokens(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first, err := NewWorkerFence(&testRedisWorkerFenceBackend{client: client}, activeFenceControl("center-a"))
	require.NoError(t, err)
	require.True(t, first.WorkersEnabled())
	require.Equal(t, uint64(1), first.Token())

	_, err = NewWorkerFence(&testRedisWorkerFenceBackend{client: client}, activeFenceControl("center-b"))
	require.ErrorIs(t, err, ErrWorkerFenceHeld)
	require.NoError(t, first.Stop())

	second, err := NewWorkerFence(&testRedisWorkerFenceBackend{client: client}, activeFenceControl("center-b"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Stop() })
	require.Greater(t, second.Token(), uint64(1))
}

func TestWorkerFenceLosesReadinessWhenLeaseOwnershipChanges(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	control := activeFenceControl("center-a")
	fence, err := NewWorkerFence(&testRedisWorkerFenceBackend{client: client}, control)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fence.Stop() })

	require.NoError(t, client.Set(context.Background(), control.WorkerLeaseKey, "center-b|999", time.Second).Err())
	select {
	case <-fence.Lost():
	case <-time.After(time.Second):
		t.Fatal("worker fence did not report lost ownership")
	}
	require.False(t, fence.Valid())
	require.False(t, fence.WorkersEnabled())
}

var testAcquireWorkerFenceScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  return {0, 0}
end
local token = redis.call("INCR", KEYS[2])
redis.call("SET", KEYS[1], ARGV[1] .. "|" .. token, "PX", ARGV[2])
return {1, token}
`)

var testRenewWorkerFenceScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
  return 1
end
return 0
`)

var testReleaseWorkerFenceScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type testRedisWorkerFenceBackend struct {
	client redis.UniversalClient
}

func (b *testRedisWorkerFenceBackend) Acquire(ctx context.Context, leaseKey, epochKey, owner string, ttl time.Duration) (int64, bool, error) {
	result, err := testAcquireWorkerFenceScript.Run(ctx, b.client, []string{leaseKey, epochKey}, owner, ttl.Milliseconds()).Slice()
	if err != nil {
		return 0, false, err
	}
	if len(result) != 2 {
		return 0, false, fmt.Errorf("unexpected acquire result length %d", len(result))
	}
	acquired, err := testRedisInteger(result[0])
	if err != nil {
		return 0, false, err
	}
	token, err := testRedisInteger(result[1])
	if err != nil {
		return 0, false, err
	}
	return token, acquired == 1, nil
}

func (b *testRedisWorkerFenceBackend) Renew(ctx context.Context, leaseKey, value string, ttl time.Duration) (bool, error) {
	result, err := testRenewWorkerFenceScript.Run(ctx, b.client, []string{leaseKey}, value, ttl.Milliseconds()).Int()
	return result == 1, err
}

func (b *testRedisWorkerFenceBackend) Release(ctx context.Context, leaseKey, value string) error {
	return testReleaseWorkerFenceScript.Run(ctx, b.client, []string{leaseKey}, value).Err()
}

func testRedisInteger(value any) (int64, error) {
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

func TestWorkerFenceDisablesWorkersForStandbyWithoutRedis(t *testing.T) {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleStandby
	control.InstanceID = "center-standby"

	fence, err := NewWorkerFence(nil, control)
	require.NoError(t, err)
	require.True(t, fence.Valid())
	require.False(t, fence.WorkersEnabled())
	require.False(t, fence.Required())
	require.NoError(t, fence.Stop())
}
