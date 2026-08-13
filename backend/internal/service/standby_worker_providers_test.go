//go:build unit

package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/stretchr/testify/require"
)

type codexSyncStartProbeRepo struct {
	SettingRepository
	started chan struct{}
}

func (r *codexSyncStartProbeRepo) Get(context.Context, string) (*Setting, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	return nil, nil
}

func newStandbyWorkerFenceForTest(t *testing.T) *WorkerFence {
	t.Helper()
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleStandby
	control.InstanceID = "center-standby"
	fence, err := NewWorkerFence(nil, control)
	require.NoError(t, err)
	return fence
}

func TestStandbyProvidersDoNotStartBackgroundWorkers(t *testing.T) {
	fence := newStandbyWorkerFenceForTest(t)
	cfg := &config.Config{}
	cfg.SubscriptionMaintenance.WorkerCount = 1
	cfg.SubscriptionMaintenance.QueueSize = 1
	cfg.Pricing = config.PricingConfig{
		RemoteUpdatesEnabled: true,
		DataDir:              t.TempDir(),
		FallbackFile:         filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"),
	}

	pricing, err := ProvidePricingService(cfg, &failOnUsePricingRemoteClient{t: t}, fence)
	require.NoError(t, err)
	require.True(t, cfg.Pricing.RemoteUpdatesEnabled)
	pricingStopped := make(chan struct{})
	go func() {
		pricing.wg.Wait()
		close(pricingStopped)
	}()
	select {
	case <-pricingStopped:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("standby started pricing update scheduler")
	}
	pricing.Stop()

	emailQueue := ProvideEmailQueueService(nil, fence)
	require.True(t, emailQueue.disabled)
	require.Error(t, emailQueue.EnqueueVerifyCode("standby@example.com", "Sub2API"))
	emailQueue.Stop()
	emailQueue.Stop()

	billingCache := ProvideBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil, fence)
	require.True(t, billingCache.backgroundWorkersDisabled)
	require.Nil(t, billingCache.cacheWriteChan)
	billingCache.Stop()

	usagePool := ProvideUsageRecordWorkerPool(cfg, fence)
	require.Nil(t, usagePool.pool)
	require.Equal(t, UsageRecordSubmitModeDropped, usagePool.Submit(func(context.Context) {}))
	usagePool.Stop()

	subscription := ProvideSubscriptionService(nil, nil, billingCache, nil, cfg, fence)
	require.True(t, subscription.maintenanceDisabled)
	require.Nil(t, subscription.maintenanceQueue)
	require.Nil(t, subscription.subCacheL1)
	subscription.DoWindowMaintenance(nil)
	subscription.Stop()

	moderation := ProvideContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil, fence)
	require.True(t, moderation.workersDisabled)
	moderation.enqueueAsync(ContentModerationCheckInput{}, nil, ContentModerationInput{}, "")
	require.Zero(t, moderation.asyncEnqueued.Load())

	codexSyncStarted := make(chan struct{}, 1)
	codexSync := ProvideOpenAICodexVersionSyncService(
		&codexSyncStartProbeRepo{started: codexSyncStarted},
		&SettingService{},
		&codexVersionSyncGitHubStub{},
		fence,
	)
	select {
	case <-codexSyncStarted:
		t.Fatal("standby started Codex version synchronization")
	case <-time.After(20 * time.Millisecond):
	}
	codexSync.Stop()

	channelMonitor := ProvideChannelMonitorService(nil, nil, &SettingService{})
	channelRunner := ProvideChannelMonitorRunner(channelMonitor, &SettingService{}, fence)
	require.False(t, channelRunner.started, "standby started channel monitor runner")
	channelRunner.Stop()

	v2Aggregator := ProvideChannelMonitorV2Aggregator(&standbyChannelMonitorV2Repo{}, nil, nil, fence)
	require.Nil(t, v2Aggregator.ctx, "standby started channel monitor v2 aggregator")
	v2Aggregator.Stop()
}

type standbyChannelMonitorV2Repo struct {
	ChannelMonitorV2Repository
}
