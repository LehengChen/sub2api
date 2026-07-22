//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestCalls    int
	recentCalls    int
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	s.latestCalls++
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	s.recentCalls++
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}

func TestUpdateServiceExternalModeUsesCatalogWithoutGitHub(t *testing.T) {
	client := &updateServiceGitHubClientStub{}
	control := DeploymentControl{
		Mode: DeploymentModeExternallyManaged,
		Catalog: ReleaseCatalog{
			Source:           "frenzy-release-catalog",
			Revision:         "catalog-20260722-01",
			Version:          "0.1.163",
			AppTag:           "frenzy/app/v0.1.163-frenzy.1",
			SourceRepository: "LehengChen/sub2api",
			SourceRevision:   "0123456789abcdef0123456789abcdef01234567",
			ImageTag:         "v0.1.163-frenzy.1",
			ImageDigest:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			OpsRevision:      "abcdef0123456789abcdef0123456789abcdef01",
		},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.151", "release", control)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.ManagedExternally)
	require.Equal(t, "managed", info.CheckStatus)
	require.Equal(t, "valid", info.CatalogStatus)
	require.Equal(t, "0.1.163", info.LatestVersion)
	require.True(t, info.HasUpdate)
	require.False(t, info.Capabilities.Update)
	require.Equal(t, 0, client.latestCalls)
	require.Equal(t, 0, client.recentCalls)
}

func TestUpdateServiceExternalModeWarnsWhenCatalogIncomplete(t *testing.T) {
	control := DeploymentControl{
		Mode:    DeploymentModeExternallyManaged,
		Catalog: ReleaseCatalog{Version: "0.1.163"},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.151", "release", control)

	info, err := svc.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.Equal(t, "unconfigured", info.CheckStatus)
	require.Equal(t, "incomplete", info.CatalogStatus)
	require.NotEmpty(t, info.Warning)
	require.False(t, info.HasUpdate)
	require.Equal(t, "0.1.151", info.LatestVersion)
}

func TestUpdateServiceExternalModeRejectsMutations(t *testing.T) {
	control := DeploymentControl{Mode: DeploymentModeExternallyManaged}
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.151", "release", control)

	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrExternallyManaged)
	require.ErrorIs(t, svc.Rollback(), ErrExternallyManaged)
	_, err := svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrExternallyManaged)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "0.1.150"), ErrExternallyManaged)
}

func TestUpdateServiceCacheWarningPreventsFalseUpToDateState(t *testing.T) {
	cache := &updateServiceCacheStub{
		data: `{"latest":"0.1.151","release_info":null,"timestamp":9999999999}`,
	}
	svc := NewUpdateService(cache, &updateServiceGitHubClientStub{}, "0.1.151", "release")

	info, err := svc.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.True(t, info.Cached)
	require.Equal(t, "cached", info.CheckStatus)
	require.NotEmpty(t, info.Warning)
	require.False(t, info.HasUpdate)
}

func TestCompareVersionsUnderstandsFrenzyTagSuffix(t *testing.T) {
	require.Equal(t, -1, compareVersions("0.1.151-frenzy.1", "0.1.163"))
	require.Equal(t, 0, compareVersions("v0.1.163-frenzy.1", "0.1.163"))
}
