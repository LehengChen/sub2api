package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Admission runs before Forward. Skip busy OAuth accounts without marking
// them unhealthy, sharing the upstream switch budget and waiting only once.
type openAISlotFailover struct {
	excluded    map[int64]struct{}
	switches    *int
	maxSwitches int
	skipWait    bool
	lastError   error
}

func (f *openAISlotFailover) retry(c *gin.Context, account *service.Account, err error) bool {
	if f == nil || account == nil || !account.IsOpenAIOAuthLike() || !openAIRequestAllowsFailoverReplay(c) {
		return false
	}
	var busy *ConcurrencyError
	var full *WaitQueueFullError
	if !errors.As(err, &busy) && !errors.As(err, &full) {
		return false
	}
	if *f.switches >= f.maxSwitches {
		return false
	}
	f.excluded[account.ID] = struct{}{}
	*f.switches++
	f.skipWait = true
	f.lastError = err
	return true
}
