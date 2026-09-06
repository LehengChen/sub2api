import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import VersionBadge from '@/components/common/VersionBadge.vue'

const { appStore, performUpdate, restartService, getRollbackVersions, rollback } = vi.hoisted(() => ({
  appStore: {
    versionLoading: false,
    currentVersion: '0.1.151',
    latestVersion: '0.1.163',
    hasUpdate: true,
    releaseInfo: null,
    buildType: 'release',
    versionCached: false,
    versionWarning: '',
    deploymentMode: 'externally_managed',
    managedExternally: true,
    updateCapabilities: {
      check_updates: true,
      update: false,
      rollback: false,
      restart: false
    },
    releaseCatalog: {
      source: 'frenzy-release-catalog',
      version: '0.1.163',
      app_tag: 'frenzy/app/v0.1.163-frenzy.1',
      source_revision: '0123456789abcdef0123456789abcdef01234567',
      image_digest:
        'sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
      ops_revision: 'abcdef0123456789abcdef0123456789abcdef01'
    },
    catalogStatus: 'valid',
    fetchVersion: vi.fn().mockResolvedValue(null),
    clearVersionCache: vi.fn()
  },
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => appStore
}))

vi.mock('@/api/admin/system', () => ({
  performUpdate,
  restartService,
  getRollbackVersions,
  rollback
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() })
}))

describe('VersionBadge deployment controls', () => {
  beforeEach(() => {
    appStore.versionCached = false
    appStore.versionWarning = ''
    appStore.deploymentMode = 'externally_managed'
    appStore.managedExternally = true
    appStore.hasUpdate = true
    appStore.catalogStatus = 'valid'
    appStore.fetchVersion.mockClear()
    performUpdate.mockReset()
    restartService.mockReset()
    getRollbackVersions.mockReset()
    rollback.mockReset()
  })

  it('shows immutable catalog identity and no mutation controls when externally managed', async () => {
    const wrapper = mount(VersionBadge)

    await wrapper.get('button').trigger('click')

    expect(wrapper.text()).toContain('version.externallyManaged')
    expect(wrapper.text()).toContain('frenzy/app/v0.1.163-frenzy.1')
    expect(wrapper.text()).toContain('0123456789abcdef0123456789abcdef01234567')
    expect(wrapper.text()).toContain(
      'sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
    )
    expect(wrapper.text()).toContain('abcdef0123456789abcdef0123456789abcdef01')
    expect(wrapper.text()).not.toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.rollback')
    expect(wrapper.text()).not.toContain('version.restartNow')
    expect(performUpdate).not.toHaveBeenCalled()
    expect(restartService).not.toHaveBeenCalled()
    expect(rollback).not.toHaveBeenCalled()
  })

  it('renders a warning instead of claiming up-to-date for cached data', async () => {
    appStore.deploymentMode = 'self_managed'
    appStore.managedExternally = false
    appStore.hasUpdate = false
    appStore.versionCached = true
    appStore.versionWarning = 'Using cached data: catalog unavailable'

    const wrapper = mount(VersionBadge)
    await wrapper.get('button').trigger('click')

    expect(wrapper.text()).toContain('version.checkWarning')
    expect(wrapper.text()).toContain('Using cached data: catalog unavailable')
    expect(wrapper.text()).toContain('version.cachedWarning')
    expect(wrapper.text()).toContain('version.statusUnverified')
    expect(wrapper.text()).not.toContain('version.upToDate')
  })
})
