//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/stretchr/testify/require"
)

func TestPeriodicAccountProbesStayStoppedOnStandby(t *testing.T) {
	control := runtimecontrol.Default()
	control.Role = runtimecontrol.RoleStandby
	control.InstanceID = "center-standby"
	fence, err := NewWorkerFence(nil, control)
	require.NoError(t, err)

	billing := ProvideUpstreamBillingProbeService(nil, nil, nil, nil, nil, fence)
	require.False(t, billing.started)

	ollama := ProvideOllamaCloudUsageService(nil, nil, nil, nil, &config.Config{}, nil, nil, fence)
	require.False(t, ollama.started)
}
