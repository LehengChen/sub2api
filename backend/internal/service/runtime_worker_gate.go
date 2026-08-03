package service

import "github.com/Wei-Shaw/sub2api/internal/runtimecontrol"

// shouldStartBackgroundWorkers preserves the legacy behavior for direct
// constructors (a nil fence means the caller owns lifecycle decisions) while
// making a standby process inert when it is built through Wire.
func shouldStartBackgroundWorkers(fence *WorkerFence) bool {
	return fence == nil || fence.WorkersEnabled()
}

func isStandbyFence(fence *WorkerFence) bool {
	return fence != nil && fence.control.Role == runtimecontrol.RoleStandby
}
