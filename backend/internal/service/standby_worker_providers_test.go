//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/stretchr/testify/require"
)

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

	moderation := ProvideContentModerationService(nil, nil, nil, nil, nil, nil, nil, fence)
	require.True(t, moderation.workersDisabled)
	moderation.enqueueAsync(ContentModerationCheckInput{}, nil, ContentModerationInput{}, "")
	require.Zero(t, moderation.asyncEnqueued.Load())
}
