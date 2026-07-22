//go:build unit

package service

import (
	"context"
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

	first, err := NewWorkerFence(client, activeFenceControl("center-a"))
	require.NoError(t, err)
	require.True(t, first.WorkersEnabled())
	require.Equal(t, uint64(1), first.Token())

	_, err = NewWorkerFence(client, activeFenceControl("center-b"))
	require.ErrorIs(t, err, ErrWorkerFenceHeld)
	require.NoError(t, first.Stop())

	second, err := NewWorkerFence(client, activeFenceControl("center-b"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Stop() })
	require.Greater(t, second.Token(), uint64(1))
}

func TestWorkerFenceLosesReadinessWhenLeaseOwnershipChanges(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	control := activeFenceControl("center-a")
	fence, err := NewWorkerFence(client, control)
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
