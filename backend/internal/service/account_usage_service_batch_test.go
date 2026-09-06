package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Minimal UsageLogRepository stub for batch usage tests (HEAD lacks geminiUsageLogRepoStub).
type usageBatchLogRepoStub struct{}

var _ UsageLogRepository = (*usageBatchLogRepoStub)(nil)

func (r *usageBatchLogRepoStub) Create(context.Context, *UsageLog) (bool, error) {
	return false, nil
}
func (r *usageBatchLogRepoStub) GetByID(context.Context, int64) (*UsageLog, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) Delete(context.Context, int64) error { return nil }
func (r *usageBatchLogRepoStub) ListByUser(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAPIKey(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAccount(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByUserAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAPIKeyAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAccountAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByModelAndTimeRange(context.Context, string, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, nil
}
func (r *usageBatchLogRepoStub) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, nil
}
func (r *usageBatchLogRepoStub) GetDashboardStats(context.Context) (*usagestats.DashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUsageTrendWithFilters(context.Context, time.Time, time.Time, string, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetEndpointStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.EndpointStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUpstreamEndpointStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.EndpointStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetGroupStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.GroupStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserBreakdownStats(context.Context, time.Time, time.Time, usagestats.UserBreakdownDimension, int) ([]usagestats.UserBreakdownItem, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAllGroupUsageSummary(context.Context, time.Time) ([]usagestats.GroupUsageSummary, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyUsageTrend(context.Context, time.Time, time.Time, string, int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserUsageTrend(context.Context, time.Time, time.Time, string, int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserSpendingRanking(context.Context, time.Time, time.Time, int) (*usagestats.UserSpendingRankingResponse, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetBatchUserUsageStats(context.Context, []int64, time.Time, time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetBatchAPIKeyUsageStats(context.Context, []int64, time.Time, time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserDashboardStats(context.Context, int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyDashboardStats(context.Context, int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserUsageTrendByUserID(context.Context, int64, time.Time, time.Time, string) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserModelStats(context.Context, int64, time.Time, time.Time) ([]usagestats.ModelStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) GetGlobalStats(context.Context, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetStatsWithFilters(context.Context, usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountUsageStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetModelStatsAggregated(context.Context, string, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetDailyStatsAggregated(context.Context, int64, time.Time, time.Time) ([]map[string]any, error) {
	return nil, nil
}

type usageBatchWindowStatsRepoStub struct {
	usageBatchLogRepoStub

	mu             sync.Mutex
	startTimeCalls []map[int64]time.Time
	batchErr       error
}

func (r *usageBatchWindowStatsRepoStub) GetAccountWindowStatsBatchByStartTimes(_ context.Context, starts map[int64]time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.mu.Lock()
	call := make(map[int64]time.Time, len(starts))
	for accountID, start := range starts {
		call[accountID] = start
	}
	r.startTimeCalls = append(r.startTimeCalls, call)
	r.mu.Unlock()
	if r.batchErr != nil {
		return nil, r.batchErr
	}

	result := make(map[int64]*usagestats.AccountStats, len(starts))
	for accountID := range starts {
		result[accountID] = &usagestats.AccountStats{
			Requests: accountID,
			Tokens:   accountID * 10,
		}
	}
	return result, nil
}

func TestAccountUsageService_GetUsageBatch_BestEffortByAccount(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	repo := &stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:       7001,
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"passive_usage_7d_utilization": 0.62,
				},
			},
			{
				ID:       7002,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"codex_usage_updated_at":  time.Now().UTC().Format(time.RFC3339),
					"codex_5h_used_percent":   18.0,
					"codex_5h_reset_at":       resetAt.Format(time.RFC3339),
					"codex_7d_used_percent":   34.0,
					"codex_7d_reset_at":       resetAt.Add(24 * time.Hour).Format(time.RFC3339),
					"workspace_id":            "org-test",
					"chatgpt_account_id":      "acct-test",
					"openai_snapshot_version": "test",
				},
			},
			{
				ID:       7003,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			},
		},
	}

	svc := &AccountUsageService{
		accountRepo:  repo,
		usageLogRepo: &usageBatchLogRepoStub{},
		cache:        NewUsageCache(),
	}

	usageByAccount, errorsByAccount, err := svc.GetUsageBatch(context.Background(), []int64{7001, 7002, 7003, 7002}, false)
	if err != nil {
		t.Fatalf("GetUsageBatch() error = %v", err)
	}

	if usageByAccount[7001] == nil || usageByAccount[7001].Source != "passive" {
		t.Fatalf("expected anthropic passive usage, got %#v", usageByAccount[7001])
	}

	if usageByAccount[7002] == nil || usageByAccount[7002].FiveHour == nil || usageByAccount[7002].FiveHour.Utilization != 18.0 {
		t.Fatalf("expected openai snapshot usage, got %#v", usageByAccount[7002])
	}

	if !strings.Contains(strings.ToLower(errorsByAccount[7003]), "does not support usage query") {
		t.Fatalf("expected API key account error to be preserved, got %q", errorsByAccount[7003])
	}
}

func TestAccountUsageService_GetUsageBatch_BatchesOpenAIWindowStats(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	firstReset := now.Add(2 * time.Hour).Truncate(time.Second)
	secondReset := now.Add(3 * time.Hour).Truncate(time.Second)
	accounts := []Account{
		{
			ID:       8001,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
				"codex_5h_used_percent":  10.0,
				"codex_5h_reset_at":      firstReset.Format(time.RFC3339),
				"codex_7d_used_percent":  20.0,
				"codex_7d_reset_at":      firstReset.Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
		{
			ID:       8002,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
				"codex_5h_used_percent":  30.0,
				"codex_5h_reset_at":      secondReset.Format(time.RFC3339),
				"codex_7d_used_percent":  40.0,
				"codex_7d_reset_at":      secondReset.Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
	}
	repo := &usageBatchWindowStatsRepoStub{}
	accountRepo := &stubOpenAIAccountRepo{accounts: accounts}
	svc := &AccountUsageService{
		accountRepo:  accountRepo,
		usageLogRepo: repo,
		cache:        NewUsageCache(),
	}

	usageByAccount, errorsByAccount, err := svc.GetUsageBatch(context.Background(), []int64{8001, 8002}, false)
	if err != nil {
		t.Fatalf("GetUsageBatch() error = %v", err)
	}
	if len(errorsByAccount) != 0 {
		t.Fatalf("unexpected per-account errors: %#v", errorsByAccount)
	}
	for _, accountID := range []int64{8001, 8002} {
		usage := usageByAccount[accountID]
		if usage == nil || usage.FiveHour == nil || usage.SevenDay == nil {
			t.Fatalf("expected both OpenAI windows for account %d, got %#v", accountID, usage)
		}
		if usage.FiveHour.WindowStats == nil || usage.FiveHour.WindowStats.Requests != accountID {
			t.Fatalf("expected batched 5h stats for account %d, got %#v", accountID, usage.FiveHour.WindowStats)
		}
		if usage.SevenDay.WindowStats == nil || usage.SevenDay.WindowStats.Requests != accountID {
			t.Fatalf("expected batched 7d stats for account %d, got %#v", accountID, usage.SevenDay.WindowStats)
		}
	}

	repo.mu.Lock()
	calls := append([]map[int64]time.Time(nil), repo.startTimeCalls...)
	repo.mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected one batch query per window, got %d", len(calls))
	}
	if len(calls[0]) != 2 || len(calls[1]) != 2 {
		t.Fatalf("expected both accounts in each batch query, got %#v", calls)
	}
	if got := calls[0][8001]; got.Sub(firstReset.Add(-5*time.Hour)) > time.Second || got.Sub(firstReset.Add(-5*time.Hour)) < -time.Second {
		t.Fatalf("5h start for account 8001 = %v, want near %v", got, firstReset.Add(-5*time.Hour))
	}
	if got := calls[0][8002]; got.Sub(secondReset.Add(-5*time.Hour)) > time.Second || got.Sub(secondReset.Add(-5*time.Hour)) < -time.Second {
		t.Fatalf("5h start for account 8002 = %v, want near %v", got, secondReset.Add(-5*time.Hour))
	}
}

func TestAccountUsageService_GetUsageBatch_WindowStatsFailureKeepsQuotaSnapshot(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	account := Account{
		ID:       8003,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
			"codex_5h_used_percent":  55.0,
			"codex_5h_reset_at":      resetAt.Format(time.RFC3339),
			"codex_7d_used_percent":  65.0,
			"codex_7d_reset_at":      resetAt.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	repo := &usageBatchWindowStatsRepoStub{batchErr: context.Canceled}
	svc := &AccountUsageService{
		accountRepo:  &stubOpenAIAccountRepo{accounts: []Account{account}},
		usageLogRepo: repo,
		cache:        NewUsageCache(),
	}

	usageByAccount, errorsByAccount, err := svc.GetUsageBatch(context.Background(), []int64{account.ID}, false)
	if err != nil {
		t.Fatalf("GetUsageBatch() error = %v", err)
	}
	if len(errorsByAccount) != 0 {
		t.Fatalf("window stats failure should not become an account error: %#v", errorsByAccount)
	}
	usage := usageByAccount[account.ID]
	if usage == nil || usage.FiveHour == nil || usage.FiveHour.Utilization != 55.0 {
		t.Fatalf("expected quota snapshot despite stats failure, got %#v", usage)
	}
}
