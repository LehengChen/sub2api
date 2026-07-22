//go:build unit

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type oauthSessionFixture struct {
	State string `json:"state"`
}

func TestProvideOAuthSessionStore_TwoInstancesConsumeExactlyOnce(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := newRuntimeSharedStateTestClient(t, server.Addr())
	clientB := newRuntimeSharedStateTestClient(t, server.Addr())

	storeA, err := ProvideOAuthSessionStore(clientA)
	require.NoError(t, err)
	storeB, err := ProvideOAuthSessionStore(clientB)
	require.NoError(t, err)

	ctx := context.Background()
	want := oauthSessionFixture{State: "opaque-state"}
	require.NoError(t, storeA.Save(ctx, service.OAuthSessionProviderOpenAI, "shared-session", want, time.Minute))

	type consumeResult struct {
		value oauthSessionFixture
		err   error
	}
	start := make(chan struct{})
	results := make(chan consumeResult, 2)
	var workers sync.WaitGroup
	for _, store := range []service.OAuthSessionStore{storeA, storeB} {
		workers.Add(1)
		go func(store service.OAuthSessionStore) {
			defer workers.Done()
			<-start
			var value oauthSessionFixture
			err := store.Consume(ctx, service.OAuthSessionProviderOpenAI, "shared-session", &value)
			results <- consumeResult{
				value: value,
				err:   err,
			}
		}(store)
	}
	close(start)
	workers.Wait()
	close(results)

	successes := 0
	notFound := 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			require.Equal(t, want, result.value)
		case errors.Is(result.err, service.ErrOAuthSessionNotFound):
			notFound++
		default:
			t.Fatalf("unexpected consume result: %v", result.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, notFound)
}

func TestRedisOAuthSessionBackend_TTLAndNotFound(t *testing.T) {
	server := miniredis.RunT(t)
	client := newRuntimeSharedStateTestClient(t, server.Addr())
	backend := &redisOAuthSessionBackend{client: client}
	ctx := context.Background()
	const key = "test:oauth:ttl"

	require.NoError(t, backend.Set(ctx, key, []byte(`{"state":"opaque"}`), time.Minute))
	payload, err := backend.Get(ctx, key)
	require.NoError(t, err)
	require.JSONEq(t, `{"state":"opaque"}`, string(payload))

	server.FastForward(2 * time.Minute)
	_, err = backend.Get(ctx, key)
	require.ErrorIs(t, err, service.ErrOAuthSessionBackendNotFound)
	_, err = backend.Consume(ctx, key)
	require.ErrorIs(t, err, service.ErrOAuthSessionBackendNotFound)
}

func TestProvideOAuthSessionStore_RedisDisconnectFailsClosed(t *testing.T) {
	server := miniredis.RunT(t)
	client := newRuntimeSharedStateTestClient(t, server.Addr())
	store, err := ProvideOAuthSessionStore(client)
	require.NoError(t, err)
	require.NoError(t, client.Close())

	ctx := context.Background()
	err = store.Save(
		ctx,
		service.OAuthSessionProviderOpenAI,
		"unavailable-session",
		oauthSessionFixture{State: "opaque-state"},
		time.Minute,
	)
	require.ErrorIs(t, err, service.ErrOAuthSessionStoreUnavailable)

	var value oauthSessionFixture
	err = store.Load(ctx, service.OAuthSessionProviderOpenAI, "unavailable-session", &value)
	require.ErrorIs(t, err, service.ErrOAuthSessionStoreUnavailable)
	err = store.Consume(ctx, service.OAuthSessionProviderOpenAI, "unavailable-session", &value)
	require.ErrorIs(t, err, service.ErrOAuthSessionStoreUnavailable)
	err = store.Delete(ctx, service.OAuthSessionProviderOpenAI, "unavailable-session")
	require.ErrorIs(t, err, service.ErrOAuthSessionStoreUnavailable)
}

func TestProvideWorkerFence_AllowsOnlyOneActiveAndIncrementsToken(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := newRuntimeSharedStateTestClient(t, server.Addr())
	clientB := newRuntimeSharedStateTestClient(t, server.Addr())

	primary, err := ProvideWorkerFence(clientA, runtimeSharedStateTestControl("center-a"))
	require.NoError(t, err)
	require.True(t, primary.Valid())
	require.True(t, primary.WorkersEnabled())
	firstToken := primary.Token()
	require.Positive(t, firstToken)

	_, err = ProvideWorkerFence(clientB, runtimeSharedStateTestControl("center-b"))
	require.ErrorIs(t, err, service.ErrWorkerFenceHeld)

	require.NoError(t, primary.Stop())
	standby, err := ProvideWorkerFence(clientB, runtimeSharedStateTestControl("center-b"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, standby.Stop()) })
	require.Greater(t, standby.Token(), firstToken)
}

func TestRedisWorkerFenceBackend_StaleOwnerCannotRenewOrReleaseNewLease(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := newRuntimeSharedStateTestClient(t, server.Addr())
	clientB := newRuntimeSharedStateTestClient(t, server.Addr())
	backendA := &redisWorkerFenceBackend{client: clientA}
	backendB := &redisWorkerFenceBackend{client: clientB}
	ctx := context.Background()
	const (
		leaseKey = "test:runtime:worker"
		epochKey = leaseKey + ":epoch"
		ownerA   = "center-a"
		ownerB   = "center-b"
	)

	firstToken, acquired, err := backendA.Acquire(ctx, leaseKey, epochKey, ownerA, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Positive(t, firstToken)

	token, acquired, err := backendB.Acquire(ctx, leaseKey, epochKey, ownerB, time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)
	require.Zero(t, token)

	server.FastForward(2 * time.Minute)
	secondToken, acquired, err := backendB.Acquire(ctx, leaseKey, epochKey, ownerB, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Greater(t, secondToken, firstToken)

	oldLeaseValue := fmt.Sprintf("%s|%d", ownerA, firstToken)
	newLeaseValue := fmt.Sprintf("%s|%d", ownerB, secondToken)
	owned, err := backendA.Renew(ctx, leaseKey, oldLeaseValue, time.Minute)
	require.NoError(t, err)
	require.False(t, owned)

	require.NoError(t, backendA.Release(ctx, leaseKey, oldLeaseValue))
	leaseValue, err := clientB.Get(ctx, leaseKey).Result()
	require.NoError(t, err)
	require.Equal(t, newLeaseValue, leaseValue)
	owned, err = backendB.Renew(ctx, leaseKey, newLeaseValue, time.Minute)
	require.NoError(t, err)
	require.True(t, owned)
}

func TestRuntimeSharedStateProviders_RejectNilRedisClient(t *testing.T) {
	store, err := ProvideOAuthSessionStore(nil)
	require.Error(t, err)
	require.Nil(t, store)

	fence, err := ProvideWorkerFence(nil, runtimeSharedStateTestControl("center-a"))
	require.Error(t, err)
	require.Nil(t, fence)
}

func newRuntimeSharedStateTestClient(t *testing.T, address string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func runtimeSharedStateTestControl(instanceID string) runtimecontrol.Control {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleActive
	control.InstanceID = instanceID
	control.WorkerLeaseKey = "test:runtime:primary-worker"
	control.WorkerLeaseTTL = time.Minute
	control.WorkerRenewInterval = 10 * time.Second
	return control
}
