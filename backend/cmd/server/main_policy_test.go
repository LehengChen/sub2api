//go:build unit

package main

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestValidateStartupPolicyBlocksExternalLegacySetupBeforeWrites(t *testing.T) {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleAll
	control.InstanceID = "center-primary"
	managed := service.DeploymentControl{Mode: service.DeploymentModeExternallyManaged}

	require.Error(t, validateStartupPolicy(control, managed, false, false))
	control.Role = runtimecontrol.RoleActive
	require.Error(t, validateStartupPolicy(control, managed, true, false))
	require.Error(t, validateStartupPolicy(control, managed, false, true))
	require.NoError(t, validateStartupPolicy(control, managed, false, false))
	control.Role = runtimecontrol.RoleMigrator
	require.NoError(t, validateStartupPolicy(control, managed, false, true))
}

func TestValidateStartupPolicyAllowsSelfManagedSetup(t *testing.T) {
	control := runtimecontrol.Default()
	control.InstanceID = "local"
	require.NoError(t, validateStartupPolicy(control, service.DefaultDeploymentControl(), true, false))
}

func TestLegacyRunnerCannotActivateExternalImage(t *testing.T) {
	control := runtimecontrol.Default()
	control.InstanceID = "legacy-center"
	managed := service.DeploymentControl{Mode: service.DeploymentModeExternallyManaged}

	err := validateStartupPolicy(control, managed, false, false)

	require.ErrorContains(t, err, "upgrade legacy runners before activating this image")
	require.True(t, control.AppliesMigrations(), "legacy all would combine application activation with migrations")
	require.True(t, control.AllowsBootstrapWrites(), "legacy all would retain bootstrap write capability")
}

func TestExplicitActiveRoleIsSafeForExternalImageActivation(t *testing.T) {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleActive
	control.InstanceID = "center-primary"
	control.WorkerLeaseTTL = 60 * time.Second
	control.WorkerRenewInterval = 15 * time.Second
	managed := service.DeploymentControl{Mode: service.DeploymentModeExternallyManaged}

	require.NoError(t, control.Validate())
	require.NoError(t, validateStartupPolicy(control, managed, false, false))
	require.True(t, control.TrafficEligible())
	require.True(t, control.RequiresWorkerLease())
	require.False(t, control.AppliesMigrations())
	require.False(t, control.AllowsBootstrapWrites())
}
