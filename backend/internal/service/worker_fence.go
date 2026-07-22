package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/redis/go-redis/v9"
)

var ErrWorkerFenceHeld = errors.New("singleton worker lease is already held")

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

// WorkerFence is a single-active worker lease. It prevents an accidental
// second active/worker process from starting singleton jobs and exposes a
// monotonically increasing fencing token for audit and future write fencing.
// Automatic failover remains disabled until critical writes validate the token.
type WorkerFence struct {
	control runtimecontrol.Control
	client  redis.UniversalClient
	value   string
	token   uint64
	enabled bool
	valid   atomic.Bool
	lost    chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc
	stop    sync.Once
	lostOne sync.Once
}

func ProvideWorkerFence(redisClient *redis.Client, control runtimecontrol.Control) (*WorkerFence, error) {
	return NewWorkerFence(redisClient, control)
}

func NewWorkerFence(client redis.UniversalClient, control runtimecontrol.Control) (*WorkerFence, error) {
	if err := control.Validate(); err != nil {
		return nil, err
	}
	fence := &WorkerFence{
		control: control,
		client:  client,
		enabled: control.RunsWorkers(),
		lost:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if !control.RequiresWorkerLease() {
		fence.valid.Store(true)
		close(fence.done)
		return fence, nil
	}
	if client == nil {
		return nil, errors.New("worker fence Redis client is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), control.WorkerRenewInterval)
	defer cancel()
	result, err := acquireWorkerFenceScript.Run(
		ctx,
		client,
		[]string{control.WorkerLeaseKey, control.WorkerLeaseKey + ":epoch"},
		control.InstanceID,
		control.WorkerLeaseTTL.Milliseconds(),
	).Slice()
	if err != nil {
		return nil, fmt.Errorf("acquire singleton worker lease: %w", err)
	}
	if len(result) != 2 || integerResult(result[0]) != 1 {
		return nil, ErrWorkerFenceHeld
	}
	token := integerResult(result[1])
	if token <= 0 {
		return nil, errors.New("acquire singleton worker lease returned an invalid fencing token")
	}
	fence.token = uint64(token)
	fence.value = control.InstanceID + "|" + strconv.FormatUint(fence.token, 10)
	fence.valid.Store(true)

	renewCtx, renewCancel := context.WithCancel(context.Background())
	fence.cancel = renewCancel
	go fence.renewLoop(renewCtx)
	return fence, nil
}

func integerResult(value any) int64 {
	switch value := value.(type) {
	case int64:
		return value
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(value), 10, 64)
		return parsed
	default:
		return 0
	}
}

func (f *WorkerFence) renewLoop(ctx context.Context) {
	defer close(f.done)
	ticker := time.NewTicker(f.control.WorkerRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(context.Background(), f.control.WorkerRenewInterval)
			result, err := renewWorkerFenceScript.Run(
				renewCtx,
				f.client,
				[]string{f.control.WorkerLeaseKey},
				f.value,
				f.control.WorkerLeaseTTL.Milliseconds(),
			).Int()
			cancel()
			if err != nil || result != 1 {
				f.markLost()
				return
			}
		}
	}
}

func (f *WorkerFence) markLost() {
	if f == nil {
		return
	}
	f.valid.Store(false)
	f.lostOne.Do(func() { close(f.lost) })
}

func (f *WorkerFence) WorkersEnabled() bool {
	return f != nil && f.enabled && f.Valid()
}

func (f *WorkerFence) Valid() bool {
	return f != nil && f.valid.Load()
}

func (f *WorkerFence) Required() bool {
	return f != nil && f.control.RequiresWorkerLease()
}

func (f *WorkerFence) Token() uint64 {
	if f == nil {
		return 0
	}
	return f.token
}

func (f *WorkerFence) Lost() <-chan struct{} {
	if f == nil {
		return nil
	}
	return f.lost
}

func (f *WorkerFence) Stop() error {
	if f == nil {
		return nil
	}
	var releaseErr error
	f.stop.Do(func() {
		if f.cancel != nil {
			f.cancel()
		}
		<-f.done
		if !f.control.RequiresWorkerLease() || f.client == nil || f.value == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), f.control.WorkerRenewInterval)
		defer cancel()
		if err := releaseWorkerFenceScript.Run(ctx, f.client, []string{f.control.WorkerLeaseKey}, f.value).Err(); err != nil && !errors.Is(err, redis.Nil) {
			releaseErr = fmt.Errorf("release singleton worker lease: %w", err)
		}
		f.valid.Store(false)
	})
	return releaseErr
}

func startSingletonWorker(fence *WorkerFence, start func()) {
	if start == nil {
		return
	}
	// A nil fence preserves source compatibility for direct provider tests.
	if fence == nil || fence.WorkersEnabled() {
		start()
	}
}
