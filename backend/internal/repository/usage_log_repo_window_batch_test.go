//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAccountWindowStatsBatchByStartTimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &usageLogRepository{sql: db}
	firstStart := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	secondStart := firstStart.Add(time.Hour)
	expectation := mock.ExpectQuery(regexp.QuoteMeta("WITH account_windows(account_id, start_time)"))
	expectation.WithArgs(int64(7), firstStart, int64(42), secondStart).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard_cost", "user_cost"}).
			AddRow(int64(7), int64(3), int64(300), 1.25, 1.0, 0.9))

	stats, err := repo.GetAccountWindowStatsBatchByStartTimes(context.Background(), map[int64]time.Time{
		42: secondStart,
		7:  firstStart,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), stats[7].Requests)
	require.Equal(t, int64(300), stats[7].Tokens)
	require.Equal(t, int64(0), stats[42].Requests)
	require.NoError(t, mock.ExpectationsWereMet())
}
