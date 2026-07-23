package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"regexp"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCheckMigrationsFSReadyAndToleratesNewerRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	fsys := fstest.MapFS{
		"001_init.sql":  {Data: []byte("CREATE TABLE example (id BIGINT PRIMARY KEY);")},
		"002_empty.sql": {Data: []byte("  \n")},
	}
	checksum := readinessTestChecksum(t, fsys, "001_init.sql")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT filename, checksum FROM schema_migrations")).
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}).
			AddRow("001_init.sql", checksum).
			AddRow("003_from_newer_release.sql", "newer-checksum"))

	require.NoError(t, checkMigrationsFS(context.Background(), db, fsys))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNMinusOneReadinessToleratesV163MigrationRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const nMinusOneMigration = "173_allow_cyber_blocked_usage_request_type.sql"
	fsys := fstest.MapFS{
		nMinusOneMigration: {Data: []byte("SELECT 1;")},
	}
	rows := sqlmock.NewRows([]string{"filename", "checksum"}).
		AddRow(nMinusOneMigration, readinessTestChecksum(t, fsys, nMinusOneMigration))
	for _, name := range []string{
		"174_add_usage_log_long_context_billing.sql",
		"174_add_usage_logs_api_key_latest_ip_index_notx.sql",
		"174_group_web_search_price_per_call.sql",
		"175_add_ops_system_logs_host.sql",
		"175_default_openai_long_context_billing.sql",
		"175a_add_ops_system_logs_host_index_notx.sql",
		"176_channel_monitor_grok_provider.sql",
		"177_add_subscription_plan_currency.sql",
		"178_channel_image_input_price.sql",
		"179_usage_log_image_input_tokens.sql",
		"180_audit_logs.sql",
		"181_group_duplicate_operation_id.sql",
		"181_prompt_audit.sql",
		"182_prompt_audit_full_prompt.sql",
		"183_ops_ingress_reject_aggregates.sql",
		"184_auth_cache_invalidation_outbox.sql",
		"185_group_reasoning_effort_policy.sql",
	} {
		rows.AddRow(name, "newer-release-checksum")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT filename, checksum FROM schema_migrations")).
		WillReturnRows(rows)

	require.NoError(t, checkMigrationsFS(context.Background(), db, fsys))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckMigrationsFSRejectsMissingMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	fsys := fstest.MapFS{
		"001_init.sql": {Data: []byte("SELECT 1;")},
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT filename, checksum FROM schema_migrations")).
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}))

	err = checkMigrationsFS(context.Background(), db, fsys)
	require.ErrorContains(t, err, "001_init.sql is not applied")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckMigrationsFSRejectsChecksumMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	fsys := fstest.MapFS{
		"001_init.sql": {Data: []byte("SELECT 1;")},
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT filename, checksum FROM schema_migrations")).
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}).
			AddRow("001_init.sql", "unexpected"))

	err = checkMigrationsFS(context.Background(), db, fsys)
	require.ErrorContains(t, err, "001_init.sql checksum mismatch")
	require.NotContains(t, err.Error(), "unexpected", "readiness errors must not echo stored checksums")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckMigrationsFSRejectsNilDatabase(t *testing.T) {
	require.ErrorContains(t, checkMigrationsFS(context.Background(), nil, fstest.MapFS{}), "nil sql db")
}

func readinessTestChecksum(t *testing.T, fsys fs.FS, name string) string {
	t.Helper()
	content, err := fs.ReadFile(fsys, name)
	require.NoError(t, err)
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
