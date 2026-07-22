//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func clearDeploymentControlEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		deploymentControlModeEnv,
		catalogSourceEnv,
		catalogRevisionEnv,
		catalogVersionEnv,
		appTagEnv,
		sourceRepositoryEnv,
		sourceRevisionEnv,
		imageTagEnv,
		imageDigestEnv,
		opsRevisionEnv,
	} {
		t.Setenv(key, "")
	}
}

func setCompleteCatalogEnv(t *testing.T) {
	t.Helper()
	t.Setenv(catalogSourceEnv, "frenzy-release-catalog")
	t.Setenv(catalogRevisionEnv, "catalog-20260722-01")
	t.Setenv(catalogVersionEnv, "0.1.163")
	t.Setenv(appTagEnv, "frenzy/app/v0.1.163-frenzy.1")
	t.Setenv(sourceRepositoryEnv, "LehengChen/sub2api")
	t.Setenv(sourceRevisionEnv, "0123456789abcdef0123456789abcdef01234567")
	t.Setenv(imageTagEnv, "v0.1.163-frenzy.1")
	t.Setenv(imageDigestEnv, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv(opsRevisionEnv, "abcdef0123456789abcdef0123456789abcdef01")
}

func TestLoadDeploymentControlFromEnvDefaultsToSelfManaged(t *testing.T) {
	clearDeploymentControlEnv(t)

	control, err := LoadDeploymentControlFromEnv()

	require.NoError(t, err)
	require.Equal(t, DeploymentModeSelfManaged, control.Mode)
	require.False(t, control.IsExternallyManaged())
	require.True(t, control.Capabilities().Update)
}

func TestLoadDeploymentControlFromEnvLoadsCompleteExternalCatalog(t *testing.T) {
	clearDeploymentControlEnv(t)
	t.Setenv(deploymentControlModeEnv, DeploymentModeExternallyManaged)
	setCompleteCatalogEnv(t)

	control, err := LoadDeploymentControlFromEnv()

	require.NoError(t, err)
	require.True(t, control.IsExternallyManaged())
	require.True(t, control.Catalog.Complete())
	require.Equal(t, "0.1.163", control.Catalog.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", control.Catalog.SourceRevision)
	require.Equal(t, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", control.Catalog.ImageDigest)
	require.Equal(t, UpdateCapabilities{
		CheckUpdates: true,
		Update:       false,
		Rollback:     false,
		Restart:      false,
	}, control.Capabilities())
}

func TestLoadDeploymentControlFromEnvRejectsUnknownMode(t *testing.T) {
	clearDeploymentControlEnv(t)
	t.Setenv(deploymentControlModeEnv, "external-ish")

	_, err := LoadDeploymentControlFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), deploymentControlModeEnv)
}

func TestLoadDeploymentControlFromEnvRejectsShortSourceRevision(t *testing.T) {
	clearDeploymentControlEnv(t)
	setCompleteCatalogEnv(t)
	t.Setenv(sourceRevisionEnv, "01234567")

	_, err := LoadDeploymentControlFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), "full hexadecimal commit SHA")
}

func TestLoadDeploymentControlFromEnvRejectsMalformedImageDigest(t *testing.T) {
	clearDeploymentControlEnv(t)
	setCompleteCatalogEnv(t)
	t.Setenv(imageDigestEnv, "sha256:not-a-digest")

	_, err := LoadDeploymentControlFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), "sha256 digest")
}

func TestExternalDeploymentAllowsIncompleteCatalogButNeverMutation(t *testing.T) {
	clearDeploymentControlEnv(t)
	t.Setenv(deploymentControlModeEnv, DeploymentModeExternallyManaged)
	t.Setenv(catalogVersionEnv, "0.1.163")

	control, err := LoadDeploymentControlFromEnv()

	require.NoError(t, err)
	require.False(t, control.Catalog.Complete())
	require.Contains(t, control.Catalog.MissingFields(), "source_revision")
	require.False(t, control.Capabilities().Update)
	require.False(t, control.Capabilities().Rollback)
	require.False(t, control.Capabilities().Restart)
}
