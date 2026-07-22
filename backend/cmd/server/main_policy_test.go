//go:build unit

package main

import (
	"testing"

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
