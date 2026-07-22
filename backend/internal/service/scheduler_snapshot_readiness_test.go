//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotInitialReadinessClosesOnlyAfterStart(t *testing.T) {
	svc := NewSchedulerSnapshotService(nil, nil, nil, nil, nil)
	require.ErrorIs(t, svc.InitialRebuildReady(), ErrSchedulerCacheNotReady)

	svc.Start()
	require.NoError(t, svc.InitialRebuildReady())
	require.NotPanics(t, svc.Start)
	svc.Stop()
}
