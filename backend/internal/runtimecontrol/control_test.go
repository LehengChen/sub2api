//go:build unit

package runtimecontrol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func clearRuntimeEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{ProcessRoleEnv, InstanceIDEnv, WorkerLeaseKeyEnv, WorkerLeaseTTLEnv, WorkerLeaseRenewEnv, MultiAPIEnabledEnv} {
		t.Setenv(name, "")
	}
}

func TestLoadFromEnvDefaultsToLegacyAll(t *testing.T) {
	clearRuntimeEnv(t)
	control, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, RoleAll, control.Role)
	require.True(t, control.ServesHTTP())
	require.True(t, control.RunsWorkers())
	require.True(t, control.AppliesMigrations())
	require.False(t, control.RequiresWorkerLease())
}

func TestRoleCapabilities(t *testing.T) {
	tests := []struct {
		role            Role
		http            bool
		traffic         bool
		workers         bool
		lease           bool
		migrations      bool
		bootstrapWrites bool
	}{
		{RoleAll, true, true, true, false, true, true},
		{RoleActive, true, true, true, true, false, false},
		{RoleAPI, true, false, false, false, false, false},
		{RoleWorker, false, false, true, true, false, false},
		{RoleStandby, true, false, false, false, false, false},
		{RoleMigrator, false, false, false, false, true, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			control := Default()
			control.Role = tt.role
			control.InstanceID = "center-a"
			require.NoError(t, control.Validate())
			require.Equal(t, tt.http, control.ServesHTTP())
			require.Equal(t, tt.traffic, control.TrafficEligible())
			require.Equal(t, tt.workers, control.RunsWorkers())
			require.Equal(t, tt.lease, control.RequiresWorkerLease())
			require.Equal(t, tt.migrations, control.AppliesMigrations())
			require.Equal(t, tt.bootstrapWrites, control.AllowsBootstrapWrites())
		})
	}
}

func TestAPIRoleRequiresExplicitFutureCapabilityGate(t *testing.T) {
	control := Default()
	control.Role = RoleAPI
	control.InstanceID = "center-api"
	require.False(t, control.TrafficEligible())
	control.MultiAPIEnabled = true
	require.True(t, control.TrafficEligible())
}

func TestLoadFromEnvParsesActiveLease(t *testing.T) {
	clearRuntimeEnv(t)
	t.Setenv(ProcessRoleEnv, "ACTIVE")
	t.Setenv(InstanceIDEnv, "center-primary")
	t.Setenv(WorkerLeaseKeyEnv, "sub2api:test:worker")
	t.Setenv(WorkerLeaseTTLEnv, "45")
	t.Setenv(WorkerLeaseRenewEnv, "10")

	control, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, RoleActive, control.Role)
	require.Equal(t, "center-primary", control.InstanceID)
	require.Equal(t, 45*time.Second, control.WorkerLeaseTTL)
	require.Equal(t, 10*time.Second, control.WorkerRenewInterval)
}

func TestLoadFromEnvRejectsUnsafeOrInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value string
	}{
		{"unknown role", ProcessRoleEnv, "primary-ish"},
		{"unsafe instance", InstanceIDEnv, "center/a"},
		{"unsafe lease key", WorkerLeaseKeyEnv, "secret key"},
		{"invalid ttl", WorkerLeaseTTLEnv, "zero"},
		{"renew too slow", WorkerLeaseRenewEnv, "20"},
		{"invalid multi api gate", MultiAPIEnabledEnv, "enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearRuntimeEnv(t)
			t.Setenv(InstanceIDEnv, "center-a")
			t.Setenv(tc.key, tc.value)
			_, err := LoadFromEnv()
			require.Error(t, err)
		})
	}
}
