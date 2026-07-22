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
)

var ErrWorkerFenceHeld = errors.New("singleton worker lease is already held")

// WorkerFenceBackend is the shared-state port for the single-active worker
// lease. The repository layer owns the Redis scripts and translates their
// result types into these stable service-level operations.
type WorkerFenceBackend interface {
	Acquire(ctx context.Context, leaseKey, epochKey, owner string, ttl time.Duration) (token int64, acquired bool, err error)
	Renew(ctx context.Context, leaseKey, value string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, leaseKey, value string) error
}

// WorkerFence is a single-active worker lease. It prevents an accidental
// second active/worker process from starting singleton jobs and exposes a
// monotonically increasing fencing token for audit and future write fencing.
// Automatic failover remains disabled until critical writes validate the token.
type WorkerFence struct {
	control runtimecontrol.Control
	backend WorkerFenceBackend
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

func NewWorkerFence(backend WorkerFenceBackend, control runtimecontrol.Control) (*WorkerFence, error) {
	if err := control.Validate(); err != nil {
		return nil, err
	}
	fence := &WorkerFence{
		control: control,
		backend: backend,
		enabled: control.RunsWorkers(),
		lost:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if !control.RequiresWorkerLease() {
		fence.valid.Store(true)
		close(fence.done)
		return fence, nil
	}
	if backend == nil {
		return nil, errors.New("worker fence backend is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), control.WorkerRenewInterval)
	defer cancel()
	token, acquired, err := backend.Acquire(
		ctx,
		control.WorkerLeaseKey,
		control.WorkerLeaseKey+":epoch",
		control.InstanceID,
		control.WorkerLeaseTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("acquire singleton worker lease: %w", err)
	}
	if !acquired {
		return nil, ErrWorkerFenceHeld
	}
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
			owned, err := f.backend.Renew(
				renewCtx,
				f.control.WorkerLeaseKey,
				f.value,
				f.control.WorkerLeaseTTL,
			)
			cancel()
			if err != nil || !owned {
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
		if !f.control.RequiresWorkerLease() || f.backend == nil || f.value == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), f.control.WorkerRenewInterval)
		defer cancel()
		if err := f.backend.Release(ctx, f.control.WorkerLeaseKey, f.value); err != nil {
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
