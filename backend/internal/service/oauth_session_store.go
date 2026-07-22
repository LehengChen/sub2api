package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const DefaultOAuthSessionKeyPrefix = "sub2api:oauth-sessions"

const (
	OAuthSessionProviderClaude      = "claude"
	OAuthSessionProviderOpenAI      = "openai"
	OAuthSessionProviderGemini      = "gemini"
	OAuthSessionProviderAntigravity = "antigravity"
	OAuthSessionProviderGrok        = "grok"
)

var (
	ErrOAuthSessionNotFound         = errors.New("oauth session not found or expired")
	ErrOAuthSessionStoreUnavailable = errors.New("oauth session store unavailable")
)

// OAuthSessionStore stores provider-scoped OAuth state. Consume must return and
// delete a session atomically so an authorization code flow cannot be replayed.
type OAuthSessionStore interface {
	Save(ctx context.Context, provider, sessionID string, session any, ttl time.Duration) error
	Load(ctx context.Context, provider, sessionID string, destination any) error
	Consume(ctx context.Context, provider, sessionID string, destination any) error
	Delete(ctx context.Context, provider, sessionID string) error
	Close() error
}

type memoryOAuthSession struct {
	payload   []byte
	expiresAt time.Time
}

// MemoryOAuthSessionStore is the single-process compatibility implementation.
// Deployments with more than one application instance must inject the Redis
// implementation instead.
type MemoryOAuthSessionStore struct {
	mu       sync.Mutex
	sessions map[string]memoryOAuthSession
	now      func() time.Time
}

func NewMemoryOAuthSessionStore() *MemoryOAuthSessionStore {
	return &MemoryOAuthSessionStore{
		sessions: make(map[string]memoryOAuthSession),
		now:      time.Now,
	}
}

func (s *MemoryOAuthSessionStore) Save(ctx context.Context, provider, sessionID string, session any, ttl time.Duration) error {
	key, err := oauthSessionKey("", provider, sessionID)
	if err != nil {
		return err
	}
	if err := validateOAuthSessionOperation(ctx, session, ttl, true); err != nil {
		return err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode oauth session: %w", err)
	}

	s.mu.Lock()
	s.sessions[key] = memoryOAuthSession{
		payload:   append([]byte(nil), payload...),
		expiresAt: s.now().Add(ttl),
	}
	s.mu.Unlock()
	return nil
}

func (s *MemoryOAuthSessionStore) Load(ctx context.Context, provider, sessionID string, destination any) error {
	return s.read(ctx, provider, sessionID, destination, false)
}

func (s *MemoryOAuthSessionStore) Consume(ctx context.Context, provider, sessionID string, destination any) error {
	return s.read(ctx, provider, sessionID, destination, true)
}

func (s *MemoryOAuthSessionStore) read(ctx context.Context, provider, sessionID string, destination any, consume bool) error {
	key, err := oauthSessionKey("", provider, sessionID)
	if err != nil {
		return err
	}
	if err := validateOAuthSessionOperation(ctx, destination, 0, false); err != nil {
		return err
	}

	s.mu.Lock()
	entry, ok := s.sessions[key]
	if !ok || !s.now().Before(entry.expiresAt) {
		if ok {
			delete(s.sessions, key)
		}
		s.mu.Unlock()
		return ErrOAuthSessionNotFound
	}
	if consume {
		delete(s.sessions, key)
	}
	payload := append([]byte(nil), entry.payload...)
	s.mu.Unlock()

	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("decode oauth session: %w", err)
	}
	return nil
}

func (s *MemoryOAuthSessionStore) Delete(ctx context.Context, provider, sessionID string) error {
	key, err := oauthSessionKey("", provider, sessionID)
	if err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("oauth session context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
	return nil
}

func (s *MemoryOAuthSessionStore) Close() error { return nil }

// RedisOAuthSessionStore shares OAuth state across application instances. The
// caller retains ownership of the Redis client and is responsible for closing it.
type RedisOAuthSessionStore struct {
	client    redis.UniversalClient
	keyPrefix string
}

var consumeOAuthSessionScript = redis.NewScript(`
local payload = redis.call("GET", KEYS[1])
if not payload then
  return nil
end
redis.call("DEL", KEYS[1])
return payload
`)

func NewRedisOAuthSessionStore(client redis.UniversalClient, keyPrefix string) (*RedisOAuthSessionStore, error) {
	if client == nil {
		return nil, errors.New("redis oauth session client is required")
	}
	keyPrefix = strings.Trim(strings.TrimSpace(keyPrefix), ":")
	if keyPrefix == "" {
		keyPrefix = DefaultOAuthSessionKeyPrefix
	}
	return &RedisOAuthSessionStore{client: client, keyPrefix: keyPrefix}, nil
}

func (s *RedisOAuthSessionStore) Save(ctx context.Context, provider, sessionID string, session any, ttl time.Duration) error {
	key, err := oauthSessionKey(s.keyPrefix, provider, sessionID)
	if err != nil {
		return err
	}
	if err := validateOAuthSessionOperation(ctx, session, ttl, true); err != nil {
		return err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode oauth session: %w", err)
	}
	if err := s.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("%w: save failed", ErrOAuthSessionStoreUnavailable)
	}
	return nil
}

func (s *RedisOAuthSessionStore) Load(ctx context.Context, provider, sessionID string, destination any) error {
	key, err := oauthSessionKey(s.keyPrefix, provider, sessionID)
	if err != nil {
		return err
	}
	if err := validateOAuthSessionOperation(ctx, destination, 0, false); err != nil {
		return err
	}
	payload, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrOAuthSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: load failed", ErrOAuthSessionStoreUnavailable)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("decode oauth session: %w", err)
	}
	return nil
}

func (s *RedisOAuthSessionStore) Consume(ctx context.Context, provider, sessionID string, destination any) error {
	key, err := oauthSessionKey(s.keyPrefix, provider, sessionID)
	if err != nil {
		return err
	}
	if err := validateOAuthSessionOperation(ctx, destination, 0, false); err != nil {
		return err
	}
	payload, err := consumeOAuthSessionScript.Run(ctx, s.client, []string{key}).Text()
	if errors.Is(err, redis.Nil) {
		return ErrOAuthSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: consume failed", ErrOAuthSessionStoreUnavailable)
	}
	if err := json.Unmarshal([]byte(payload), destination); err != nil {
		return fmt.Errorf("decode oauth session: %w", err)
	}
	return nil
}

func (s *RedisOAuthSessionStore) Delete(ctx context.Context, provider, sessionID string) error {
	key, err := oauthSessionKey(s.keyPrefix, provider, sessionID)
	if err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("oauth session context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("%w: delete failed", ErrOAuthSessionStoreUnavailable)
	}
	return nil
}

func (s *RedisOAuthSessionStore) Close() error { return nil }

func validateOAuthSessionOperation(ctx context.Context, value any, ttl time.Duration, requireTTL bool) error {
	if ctx == nil {
		return errors.New("oauth session context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if value == nil {
		return errors.New("oauth session value is required")
	}
	if requireTTL && ttl <= 0 {
		return errors.New("oauth session ttl must be positive")
	}
	return nil
}

func oauthSessionKey(prefix, provider, sessionID string) (string, error) {
	if !validOAuthSessionKeyPart(provider, true) {
		return "", errors.New("invalid oauth session provider")
	}
	if !validOAuthSessionKeyPart(sessionID, false) {
		return "", errors.New("invalid oauth session id")
	}
	if prefix == "" {
		return provider + ":" + sessionID, nil
	}
	return prefix + ":" + provider + ":" + sessionID, nil
}

func validOAuthSessionKeyPart(value string, provider bool) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' {
			continue
		}
		if !provider && c >= 'A' && c <= 'Z' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}
