//go:build unit

package service

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type oauthSessionStoreFixture struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	CreatedAt    time.Time `json:"created_at"`
}

func TestRedisOAuthSessionStore_TwoInstancesConsumeExactlyOnce(t *testing.T) {
	server := miniredis.RunT(t)
	storeA := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	storeB := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	ctx := context.Background()

	want := oauthSessionStoreFixture{State: "state-a", CodeVerifier: "verifier-a", CreatedAt: time.Now().UTC()}
	require.NoError(t, storeA.Save(ctx, OAuthSessionProviderOpenAI, "shared-session", &want, 30*time.Minute))

	var loaded oauthSessionStoreFixture
	require.NoError(t, storeB.Load(ctx, OAuthSessionProviderOpenAI, "shared-session", &loaded))
	require.Equal(t, want.State, loaded.State)

	const consumers = 24
	var successes atomic.Int32
	errorsSeen := make(chan error, consumers)
	var wg sync.WaitGroup
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var got oauthSessionStoreFixture
			store := storeA
			if index%2 == 1 {
				store = storeB
			}
			err := store.Consume(ctx, OAuthSessionProviderOpenAI, "shared-session", &got)
			if err == nil {
				successes.Add(1)
				require.Equal(t, want.State, got.State)
				return
			}
			errorsSeen <- err
		}(i)
	}
	wg.Wait()
	close(errorsSeen)

	require.Equal(t, int32(1), successes.Load())
	for err := range errorsSeen {
		require.ErrorIs(t, err, ErrOAuthSessionNotFound)
	}
}

func TestRedisOAuthSessionStore_ProviderNamespaceAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	store := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, OAuthSessionProviderClaude, "same-id", &oauthSessionStoreFixture{State: "claude"}, time.Minute))
	require.NoError(t, store.Save(ctx, OAuthSessionProviderGemini, "same-id", &oauthSessionStoreFixture{State: "gemini"}, 10*time.Minute))

	server.FastForward(2 * time.Minute)
	var expired oauthSessionStoreFixture
	require.ErrorIs(t, store.Load(ctx, OAuthSessionProviderClaude, "same-id", &expired), ErrOAuthSessionNotFound)

	var active oauthSessionStoreFixture
	require.NoError(t, store.Consume(ctx, OAuthSessionProviderGemini, "same-id", &active))
	require.Equal(t, "gemini", active.State)
}

func TestRedisOAuthSessionStore_CorruptPayloadIsConsumed(t *testing.T) {
	server := miniredis.RunT(t)
	client := newTestRedisClient(t, server.Addr())
	store, err := NewRedisOAuthSessionStore(client, "test:oauth")
	require.NoError(t, err)
	ctx := context.Background()

	key, err := oauthSessionKey("test:oauth", OAuthSessionProviderGrok, "corrupt-session")
	require.NoError(t, err)
	require.NoError(t, client.Set(ctx, key, "{not-json", time.Minute).Err())

	var got oauthSessionStoreFixture
	require.Error(t, store.Consume(ctx, OAuthSessionProviderGrok, "corrupt-session", &got))
	require.ErrorIs(t, store.Load(ctx, OAuthSessionProviderGrok, "corrupt-session", &got), ErrOAuthSessionNotFound)
}

func TestRedisOAuthSessionStore_FailsClosedWhenRedisUnavailable(t *testing.T) {
	server := miniredis.RunT(t)
	store := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	server.Close()

	err := store.Save(context.Background(), OAuthSessionProviderOpenAI, "session", &oauthSessionStoreFixture{State: "state"}, time.Minute)
	require.ErrorIs(t, err, ErrOAuthSessionStoreUnavailable)

	var got oauthSessionStoreFixture
	err = store.Consume(context.Background(), OAuthSessionProviderOpenAI, "session", &got)
	require.ErrorIs(t, err, ErrOAuthSessionStoreUnavailable)
}

func TestMemoryOAuthSessionStore_NamespaceExpiryAndConsume(t *testing.T) {
	store := NewMemoryOAuthSessionStore()
	now := time.Now().UTC()
	store.now = func() time.Time { return now }
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, OAuthSessionProviderClaude, "same-id", &oauthSessionStoreFixture{State: "claude"}, time.Minute))
	require.NoError(t, store.Save(ctx, OAuthSessionProviderOpenAI, "same-id", &oauthSessionStoreFixture{State: "openai"}, 2*time.Minute))

	now = now.Add(90 * time.Second)
	var got oauthSessionStoreFixture
	require.ErrorIs(t, store.Load(ctx, OAuthSessionProviderClaude, "same-id", &got), ErrOAuthSessionNotFound)
	require.NoError(t, store.Consume(ctx, OAuthSessionProviderOpenAI, "same-id", &got))
	require.Equal(t, "openai", got.State)
	require.ErrorIs(t, store.Consume(ctx, OAuthSessionProviderOpenAI, "same-id", &got), ErrOAuthSessionNotFound)
}

func TestOAuthSessionStore_RejectsUnsafeKeysAndInvalidTTL(t *testing.T) {
	store := NewMemoryOAuthSessionStore()
	ctx := context.Background()
	value := &oauthSessionStoreFixture{State: "state"}

	require.Error(t, store.Save(ctx, "OpenAI", "session", value, time.Minute))
	require.Error(t, store.Save(ctx, OAuthSessionProviderOpenAI, "../session", value, time.Minute))
	require.Error(t, store.Save(ctx, OAuthSessionProviderOpenAI, "session", value, 0))
	require.Error(t, store.Save(nil, OAuthSessionProviderOpenAI, "session", value, time.Minute))
}

type crossInstanceOpenAIOAuthClient struct{}

func (c *crossInstanceOpenAIOAuthClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	return &openai.TokenResponse{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}, nil
}

func (c *crossInstanceOpenAIOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func (c *crossInstanceOpenAIOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func TestOpenAIOAuthService_RedisSessionCrossesInstances(t *testing.T) {
	server := miniredis.RunT(t)
	storeA := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	storeB := newTestRedisOAuthSessionStore(t, server.Addr(), "test:oauth")
	client := &crossInstanceOpenAIOAuthClient{}

	serviceA, err := NewOpenAIOAuthServiceWithSessionStore(nil, client, storeA)
	require.NoError(t, err)
	serviceB, err := NewOpenAIOAuthServiceWithSessionStore(nil, client, storeB)
	require.NoError(t, err)

	result, err := serviceA.GenerateAuthURL(context.Background(), nil, "", PlatformOpenAI)
	require.NoError(t, err)
	parsed, err := url.Parse(result.AuthURL)
	require.NoError(t, err)

	token, err := serviceB.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "authorization-code",
		State:     parsed.Query().Get("state"),
	})
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)

	_, err = serviceA.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "authorization-code",
		State:     parsed.Query().Get("state"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "session not found or expired")
}

func newTestRedisOAuthSessionStore(t *testing.T, address, prefix string) *RedisOAuthSessionStore {
	t.Helper()
	store, err := NewRedisOAuthSessionStore(newTestRedisClient(t, address), prefix)
	require.NoError(t, err)
	return store
}

func newTestRedisClient(t *testing.T, address string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:         address,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
		MaxRetries:   -1,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}
