package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLiveLeaseReplacesRegularSlotsAndCountsTowardLimits(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	accountAcquired, err := regular.AcquireAccountSlot(ctx, 10, 1, "regular-account")
	require.NoError(t, err)
	require.True(t, accountAcquired)
	userAcquired, err := regular.AcquireUserSlot(ctx, 20, 1, "regular-user")
	require.NoError(t, err)
	require.True(t, userAcquired)

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "live-lease", true)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, regular.ReleaseAccountSlot(ctx, 10, "regular-account"))
	require.NoError(t, regular.ReleaseUserSlot(ctx, 20, "regular-user"))

	accountCount, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount)
	userCount, err := regular.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-blocked")
	require.NoError(t, err)
	require.False(t, accountAcquired)

	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "live-lease")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, live.ReleaseLiveLease(ctx, 10, 20, 30, "live-lease"))
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-allowed")
	require.NoError(t, err)
	require.True(t, accountAcquired)
}

func TestLiveLeaseExpiresWithoutRefresh(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "expired-live", false)
	require.NoError(t, err)
	require.True(t, acquired)

	redisServer.FastForward(61 * time.Second)
	acquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-after-expiry")
	require.NoError(t, err)
	require.True(t, acquired)
	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "expired-live")
	require.NoError(t, err)
	require.False(t, refreshed)
}

func TestConcurrencySlotsAreSharedAcrossCenterClients(t *testing.T) {
	redisServer := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = clientA.Close() })
	t.Cleanup(func() { _ = clientB.Close() })

	centerA := NewConcurrencyCache(clientA, 15, 900)
	centerB := NewConcurrencyCache(clientB, 15, 900)
	ctx := context.Background()

	accountAcquired, err := centerA.AcquireAccountSlot(ctx, 1001, 1, "center-a-account")
	require.NoError(t, err)
	require.True(t, accountAcquired)
	accountAcquired, err = centerB.AcquireAccountSlot(ctx, 1001, 1, "center-b-account")
	require.NoError(t, err)
	require.False(t, accountAcquired, "center B must observe center A's account slot")

	userAcquired, err := centerA.AcquireUserSlot(ctx, 2001, 1, "center-a-user")
	require.NoError(t, err)
	require.True(t, userAcquired)
	userAcquired, err = centerB.AcquireUserSlot(ctx, 2001, 1, "center-b-user")
	require.NoError(t, err)
	require.False(t, userAcquired, "center B must observe center A's user slot")

	require.NoError(t, centerA.ReleaseAccountSlot(ctx, 1001, "center-a-account"))
	require.NoError(t, centerA.ReleaseUserSlot(ctx, 2001, "center-a-user"))
	accountAcquired, err = centerB.AcquireAccountSlot(ctx, 1001, 1, "center-b-account")
	require.NoError(t, err)
	require.True(t, accountAcquired, "released account capacity must become available to center B")
	userAcquired, err = centerB.AcquireUserSlot(ctx, 2001, 1, "center-b-user")
	require.NoError(t, err)
	require.True(t, userAcquired, "released user capacity must become available to center B")
}
