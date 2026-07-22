//go:build unit

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadinessRequiresInitializationWithoutCallingDependencies(t *testing.T) {
	var calls atomic.Int32
	probe := func(context.Context) error {
		calls.Add(1)
		return nil
	}
	health := newHealthService(time.Second, time.Second, probe, probe, probe)

	report, ready := health.Readiness(context.Background())
	require.False(t, ready)
	require.Equal(t, "not_ready", report.Status)
	require.Equal(t, "failed", report.Checks["initialization"])
	require.Equal(t, int32(0), calls.Load())
}

func TestReadinessChecksAllRequiredDependencies(t *testing.T) {
	tests := []struct {
		name       string
		postgres   error
		redis      error
		migrations error
		wantReady  bool
		failed     string
	}{
		{name: "ready", wantReady: true},
		{name: "postgres down", postgres: errors.New("db down"), failed: "postgres"},
		{name: "redis down", redis: errors.New("redis down"), failed: "redis"},
		{name: "migration missing", migrations: errors.New("migration missing"), failed: "migrations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := newHealthService(
				time.Second,
				time.Second,
				func(context.Context) error { return tt.postgres },
				func(context.Context) error { return tt.redis },
				func(context.Context) error { return tt.migrations },
			)
			health.MarkInitialized()

			report, ready := health.Readiness(context.Background())
			require.Equal(t, tt.wantReady, ready)
			if tt.wantReady {
				require.Equal(t, "ready", report.Status)
				require.Equal(t, "ok", report.Checks["postgres"])
				require.Equal(t, "ok", report.Checks["redis"])
				require.Equal(t, "ok", report.Checks["migrations"])
				return
			}
			require.Equal(t, "not_ready", report.Status)
			require.Equal(t, "failed", report.Checks[tt.failed])
		})
	}
}

func TestReadinessUsesOneBoundedProbeDeadline(t *testing.T) {
	probeSawDeadline := make(chan struct{}, 1)
	blockingProbe := func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); ok {
			probeSawDeadline <- struct{}{}
		}
		<-ctx.Done()
		return ctx.Err()
	}
	health := newHealthService(20*time.Millisecond, time.Second, blockingProbe, func(context.Context) error { return nil }, func(context.Context) error { return nil })
	health.MarkInitialized()

	started := time.Now()
	_, ready := health.Readiness(context.Background())
	require.False(t, ready)
	require.Less(t, time.Since(started), time.Second)
	require.Eventually(t, func() bool { return len(probeSawDeadline) == 1 }, time.Second, time.Millisecond)
}

func TestHealthEndpointsAndDrainGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	success := func(context.Context) error { return nil }
	health := newHealthService(time.Second, 17*time.Second, success, success, success)
	health.MarkInitialized()

	router := gin.New()
	router.Use(health.DrainGuard())
	routes.RegisterCommonRoutes(router, health.LivenessHandler, health.ReadinessHandler)
	router.GET("/work", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "accepted"}) })

	assertStatus(t, router, "/livez", http.StatusOK)
	assertStatus(t, router, "/health", http.StatusOK)
	assertStatus(t, router, "/readyz", http.StatusOK)
	assertStatus(t, router, "/work", http.StatusOK)
	require.Equal(t, 17*time.Second, health.ShutdownTimeout())

	health.BeginDrain()
	assertStatus(t, router, "/livez", http.StatusOK)
	assertStatus(t, router, "/health", http.StatusOK)
	readyResponse := assertStatus(t, router, "/readyz", http.StatusServiceUnavailable)
	require.Contains(t, readyResponse.Body.String(), `"drain":"failed"`)
	workResponse := assertStatus(t, router, "/work", http.StatusServiceUnavailable)
	require.Equal(t, "1", workResponse.Header().Get("Retry-After"))
}

func TestTrackRequestsAndLongLivedRegistryAreBoundedByDrainWait(t *testing.T) {
	health := newHealthService(time.Second, time.Second, nil, nil, nil)
	started := make(chan struct{})
	finish := make(chan struct{})
	router := gin.New()
	router.Use(health.TrackRequests())
	router.GET("/work", func(c *gin.Context) {
		close(started)
		<-finish
	})

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/work", nil))
	}()
	<-started
	require.Equal(t, 1, health.ActiveRequests())

	releaseConnection := health.RegisterLongLivedConnection()
	require.Equal(t, 1, health.ActiveConnections())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	require.ErrorIs(t, health.WaitForDrain(ctx), context.DeadlineExceeded)
	cancel()

	close(finish)
	<-requestDone
	releaseConnection()
	require.Eventually(t, func() bool {
		return health.ActiveRequests() == 0 && health.ActiveConnections() == 0
	}, time.Second, time.Millisecond)
	require.NoError(t, health.WaitForDrain(context.Background()))
}

func TestLongLivedConnectionReleaseIsIdempotent(t *testing.T) {
	health := newHealthService(time.Second, time.Second, nil, nil, nil)
	release := health.RegisterLongLivedConnection()
	release()
	release()
	require.Equal(t, 0, health.ActiveConnections())
}

func TestReadinessResponseDoesNotExposeProbeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	health := newHealthService(
		time.Second,
		time.Second,
		func(context.Context) error { return errors.New("postgres://user:secret@example.internal/database") },
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	health.MarkInitialized()
	router := gin.New()
	routes.RegisterCommonRoutes(router, health.LivenessHandler, health.ReadinessHandler)

	response := assertStatus(t, router, "/readyz", http.StatusServiceUnavailable)
	require.False(t, strings.Contains(response.Body.String(), "secret"))
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func assertStatus(t *testing.T, handler http.Handler, path string, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, want, response.Code, response.Body.String())
	return response
}
