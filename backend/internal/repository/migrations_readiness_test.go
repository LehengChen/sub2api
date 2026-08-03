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
