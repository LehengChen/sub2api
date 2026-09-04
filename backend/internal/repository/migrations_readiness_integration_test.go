//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckMigrationsAgainstAppliedSchema(t *testing.T) {
	require.NoError(t, CheckMigrations(context.Background(), integrationDB))
}
