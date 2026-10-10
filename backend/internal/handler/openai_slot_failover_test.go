//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type slotFailoverCache struct {
	helperConcurrencyCacheStub
	queueFull       bool
	waits, releases int
}

func (s *slotFailoverCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	s.waits++
	return !s.queueFull, nil
}

func (s *slotFailoverCache) DecrementAccountWaitCount(context.Context, int64) error {
	s.releases++
	return nil
}

func slotFailoverSelection(id int64) *service.AccountSelectionResult {
	return &service.AccountSelectionResult{
		Account:  &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
		WaitPlan: &service.AccountWaitPlan{AccountID: id, MaxConcurrency: 1, MaxWaiting: 1, Timeout: 20 * time.Millisecond},
	}
}

func TestOpenAISlotFailoverReselectsAfterBusyAccount(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "queue full"}[full], func(t *testing.T) {
			cache := &slotFailoverCache{queueFull: full}
			h := &OpenAIGatewayHandler{
				gatewayService:    &service.OpenAIGatewayService{},
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatComment, 5*time.Millisecond),
			}
			c, rec := newHelperTestContext(http.MethodPost, "/v1/responses")
			count, started := 0, false
			state := &openAISlotFailover{excluded: map[int64]struct{}{}, switches: &count, maxSwitches: 2}
			release, result := h.acquireResponsesAccountSlot(c, nil, "", slotFailoverSelection(1), true, &started, zap.NewNop(), state)
			require.Equal(t, openAISlotAcquireBusy, result)
			require.Nil(t, release)
			require.Contains(t, state.excluded, int64(1))
			require.Equal(t, 1, count)
			require.NotContains(t, rec.Body.String(), "error")
			if !full {
				require.True(t, started, "queue keepalives must not suppress reselection")
			}
			require.Equal(t, map[bool]int{false: 1, true: 0}[full], cache.releases)

			// A second saturated candidate must not start another full queue wait.
			_, result = h.acquireResponsesAccountSlot(c, nil, "", slotFailoverSelection(2), true, &started, zap.NewNop(), state)
			require.Equal(t, openAISlotAcquireBusy, result)
			require.Equal(t, 1, cache.waits)
			require.Equal(t, 2, count)

			// The remaining candidate has a free slot and can proceed normally.
			cache.accountSeq = []bool{true}
			release, result = h.acquireResponsesAccountSlot(c, nil, "", slotFailoverSelection(3), true, &started, zap.NewNop(), state)
			require.Equal(t, openAISlotAcquireOK, result)
			require.Nil(t, state.lastError)
			require.NotNil(t, release)
			release()
			require.Equal(t, 1, cache.accountReleaseCalls)
		})
	}
}

func TestOpenAISlotFailoverHonorsSharedBudget(t *testing.T) {
	cache := &slotFailoverCache{queueFull: true}
	h := &OpenAIGatewayHandler{gatewayService: &service.OpenAIGatewayService{}, concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)}
	c, rec := newHelperTestContext(http.MethodPost, "/v1/responses")
	count, started := 2, false // Earlier upstream failures already used the budget.
	state := &openAISlotFailover{excluded: map[int64]struct{}{}, switches: &count, maxSwitches: 2}
	_, result := h.acquireResponsesAccountSlot(c, nil, "", slotFailoverSelection(1), false, &started, zap.NewNop(), state)
	require.Equal(t, openAISlotAcquireFailed, result)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), gatewayQueueFullCode)
	require.Empty(t, state.excluded)
	require.Equal(t, 2, count)
}

func TestOpenAISlotFailoverDoesNotRetryCancellationOrStorageError(t *testing.T) {
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")
	count := 0
	state := &openAISlotFailover{excluded: map[int64]struct{}{}, switches: &count, maxSwitches: 3}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, errors.New("redis unavailable")} {
		require.False(t, state.retry(c, slotFailoverSelection(1).Account, err))
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	require.False(t, state.retry(c, slotFailoverSelection(1).Account, &ConcurrencyError{SlotType: "account", IsTimeout: true}))
	require.Zero(t, count)
	require.Empty(t, state.excluded)
}
