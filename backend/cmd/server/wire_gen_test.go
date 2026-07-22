package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProvideServiceBuildInfo(t *testing.T) {
	in := handler.BuildInfo{
		Version:   "v-test",
		BuildType: "release",
		DeploymentControl: service.DeploymentControl{
			Mode: service.DeploymentModeExternallyManaged,
		},
	}
	out := provideServiceBuildInfo(in)
	require.Equal(t, in.Version, out.Version)
	require.Equal(t, in.BuildType, out.BuildType)
	require.Equal(t, in.DeploymentControl, out.DeploymentControl)
}

func TestDrainAndShutdownIsBoundedAndMarksUnready(t *testing.T) {
	started := make(chan struct{})
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.Serve(listener) }()

	health := server.NewHealthService(nil, nil, &config.Config{Server: config.ServerConfig{ShutdownTimeoutSeconds: 1}})
	health.MarkInitialized()
	app := &Application{Server: httpServer, Health: health, Cleanup: func() {}}

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if requestErr != nil {
			return
		}
		_, _ = http.DefaultClient.Do(request)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach server")
	}

	begin := time.Now()
	drainAndShutdown(app)
	require.True(t, health.IsDraining())
	require.Less(t, time.Since(begin), 2*time.Second)
	select {
	case err := <-serveDone:
		require.ErrorIs(t, err, http.ErrServerClosed)
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("active request was not closed at the shutdown deadline")
	}
}

func TestProvideCleanup_WithMinimalDependencies_NoPanic(t *testing.T) {
	cfg := &config.Config{}

	oauthSvc := service.NewOAuthService(nil, nil)
	openAIOAuthSvc := service.NewOpenAIOAuthService(nil, nil)
	geminiOAuthSvc := service.NewGeminiOAuthService(nil, nil, nil, nil, cfg)
	antigravityOAuthSvc := service.NewAntigravityOAuthService(nil)

	tokenRefreshSvc := service.NewTokenRefreshService(
		nil,
		oauthSvc,
		openAIOAuthSvc,
		geminiOAuthSvc,
		antigravityOAuthSvc,
		nil,
		nil,
		cfg,
		nil,
	)
	accountExpirySvc := service.NewAccountExpiryService(nil, time.Second)
	proxyExpirySvc := service.NewProxyExpiryService(nil, time.Second)
	subscriptionExpirySvc := service.NewSubscriptionExpiryService(nil, time.Second)
	pricingSvc := service.NewPricingService(cfg, nil)
	emailQueueSvc := service.NewEmailQueueService(nil, 1)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	idempotencyCleanupSvc := service.NewIdempotencyCleanupService(nil, cfg)
	schedulerSnapshotSvc := service.NewSchedulerSnapshotService(nil, nil, nil, nil, cfg)
	opsSystemLogSinkSvc := service.NewOpsSystemLogSink(nil)

	cleanup := provideCleanup(
		nil, // entClient
		nil, // redis
		&service.OpsMetricsCollector{},
		&service.OpsAggregationService{},
		&service.OpsAlertEvaluatorService{},
		&service.OpsCleanupService{},
		&service.OpsScheduledReportService{},
		opsSystemLogSinkSvc,
		nil, // opsService
		nil, // opsIngressRejectAggregator
		nil, // apiKeyService
		nil, // authCacheInvalidationWorker
		schedulerSnapshotSvc,
		tokenRefreshSvc,
		accountExpirySvc,
		proxyExpirySvc,
		subscriptionExpirySvc,
		&service.UsageCleanupService{},
		idempotencyCleanupSvc,
		&service.BatchImageCleanupService{},
		nil, // batchImageWorker
		pricingSvc,
		emailQueueSvc,
		billingCacheSvc,
		&service.UsageRecordWorkerPool{},
		&service.SubscriptionService{},
		oauthSvc,
		openAIOAuthSvc,
		geminiOAuthSvc,
		antigravityOAuthSvc,
		nil, // grokOAuth
		nil, // openAIGateway
		nil, // scheduledTestRunner
		nil, // backupSvc
		nil, // paymentOrderExpiry
		nil, // channelMonitorRunner
		nil, // quotaFlusher
		nil, // upstreamBillingProbe
		nil, // auditLog
		nil, // promptAudit
	)

	require.NotPanics(t, func() {
		cleanup()
	})
}
