//go:build unit

package server

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestManualColdStandbyRoleAndFenceSequence(t *testing.T) {
	redisServer := miniredis.RunT(t)
	primaryRedis := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	standbyRedis := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = primaryRedis.Close() })
	t.Cleanup(func() { _ = standbyRedis.Close() })

	primaryControl := manualFailoverControl(runtimecontrol.RoleActive, "center-a")
	primaryFence, err := service.NewWorkerFence(primaryRedis, primaryControl)
	require.NoError(t, err)
	require.True(t, primaryFence.WorkersEnabled())
	primaryToken := primaryFence.Token()

	primaryHealth := readyHealthForControl(primaryControl, primaryFence, primaryRedis)
	assertManualFailoverReadiness(t, primaryHealth, true)

	standbyControl := manualFailoverControl(runtimecontrol.RoleStandby, "center-b")
	standbyFence, err := service.NewWorkerFence(standbyRedis, standbyControl)
	require.NoError(t, err)
	require.False(t, standbyFence.WorkersEnabled())
	standbyHealth := readyHealthForControl(standbyControl, standbyFence, standbyRedis)
	standbyReport, standbyReady := standbyHealth.Readiness(context.Background())
	require.False(t, standbyReady)
	require.Equal(t, "failed", standbyReport.Checks["role"])

	promotedControl := manualFailoverControl(runtimecontrol.RoleActive, "center-b")
	_, err = service.NewWorkerFence(standbyRedis, promotedControl)
	require.ErrorIs(t, err, service.ErrWorkerFenceHeld,
		"the standby cannot become active while the old primary owns the lease")

	primaryHealth.BeginDrain()
	drainingReport, drainingReady := primaryHealth.Readiness(context.Background())
	require.False(t, drainingReady)
	require.Equal(t, "failed", drainingReport.Checks["drain"])
	_, err = service.NewWorkerFence(standbyRedis, promotedControl)
	require.ErrorIs(t, err, service.ErrWorkerFenceHeld,
		"draining alone must not transfer singleton ownership")

	require.NoError(t, primaryFence.Stop())
	require.False(t, primaryFence.Valid())

	promotedFence, err := service.NewWorkerFence(standbyRedis, promotedControl)
	require.NoError(t, err)
	t.Cleanup(func() { _ = promotedFence.Stop() })
	require.True(t, promotedFence.WorkersEnabled())
	require.Greater(t, promotedFence.Token(), primaryToken)

	promotedHealth := readyHealthForControl(promotedControl, promotedFence, standbyRedis)
	assertManualFailoverReadiness(t, promotedHealth, true)

	_, err = service.NewWorkerFence(primaryRedis, primaryControl)
	require.ErrorIs(t, err, service.ErrWorkerFenceHeld,
		"the stopped primary cannot rejoin while the promoted primary owns the lease")
}

func manualFailoverControl(role runtimecontrol.Role, instanceID string) runtimecontrol.Control {
	control := runtimecontrol.Default()
	control.Role = role
	control.InstanceID = instanceID
	control.WorkerLeaseKey = "sub2api:test:manual-cold-standby"
	control.WorkerLeaseTTL = 3 * time.Second
	control.WorkerRenewInterval = 500 * time.Millisecond
	return control
}

func readyHealthForControl(control runtimecontrol.Control, fence *service.WorkerFence, redisClient redis.UniversalClient) *HealthService {
	// This test isolates role/fence sequencing. PostgreSQL, migrations, and
	// scheduler use success probes and remain separate release rehearsal gates.
	success := func(context.Context) error { return nil }
	redisProbe := func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }
	health := newHealthService(time.Second, time.Second, success, redisProbe, success)
	health.runtimeControl = control
	health.workerFence = fence
	health.schedulerProbe = success
	health.MarkInitialized()
	return health
}

func assertManualFailoverReadiness(t *testing.T, health *HealthService, wantReady bool) {
	t.Helper()
	report, ready := health.Readiness(context.Background())
	require.Equal(t, wantReady, ready, report)
	if !wantReady {
		return
	}
	require.Equal(t, "ready", report.Status)
	for _, check := range []string{"initialization", "drain", "role", "worker_fence", "scheduler", "postgres", "redis", "migrations"} {
		require.Equal(t, "ok", report.Checks[check], "check %s", check)
	}
}
