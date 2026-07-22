package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
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
	// ErrOAuthSessionBackendNotFound is returned by a shared-state adapter when
	// the requested key does not exist. Infrastructure adapters translate their
	// native not-found error to this sentinel before it reaches the service.
	ErrOAuthSessionBackendNotFound = errors.New("oauth session backend entry not found")
)

// OAuthSessionBackend is the small shared-state port required by the OAuth
// session service. Redis and its atomic consume script live in the repository
// layer so service code does not depend on a concrete infrastructure client.
type OAuthSessionBackend interface {
	Set(ctx context.Context, key string, payload []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Consume(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

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
// caller retains ownership of the injected backend and its lifecycle.
type RedisOAuthSessionStore struct {
	backend   OAuthSessionBackend
	keyPrefix string
}

var (
	_ OAuthSessionStore = (*MemoryOAuthSessionStore)(nil)
	_ OAuthSessionStore = (*RedisOAuthSessionStore)(nil)
)

func NewRedisOAuthSessionStore(backend OAuthSessionBackend, keyPrefix string) (*RedisOAuthSessionStore, error) {
	if backend == nil {
		return nil, errors.New("oauth session backend is required")
	}
	keyPrefix = strings.Trim(strings.TrimSpace(keyPrefix), ":")
	if keyPrefix == "" {
		keyPrefix = DefaultOAuthSessionKeyPrefix
	}
	return &RedisOAuthSessionStore{backend: backend, keyPrefix: keyPrefix}, nil
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
	if err := s.backend.Set(ctx, key, payload, ttl); err != nil {
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
	payload, err := s.backend.Get(ctx, key)
	if errors.Is(err, ErrOAuthSessionBackendNotFound) {
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
	payload, err := s.backend.Consume(ctx, key)
	if errors.Is(err, ErrOAuthSessionBackendNotFound) {
		return ErrOAuthSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: consume failed", ErrOAuthSessionStoreUnavailable)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
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
	if err := s.backend.Delete(ctx, key); err != nil {
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
