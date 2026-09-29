import { Link, useLocation } from 'react-router-dom'
import { ArrowUpRight, Compass, Menu, Search } from 'lucide-react'

import type { PlayProfile, User } from '../types'
import { LayoutSearchBox } from './LayoutSearchBox'
import { LayoutThemeToggle } from './LayoutThemeToggle'
import { LayoutUserMenu } from './LayoutUserMenu'
import type { useLayoutProfiles } from './useLayoutProfiles'
import type { useLayoutSearch } from './useLayoutSearch'
import type { ThemeMode, useThemeMode } from './useThemeMode'
import { LAYOUT_NAV_GROUPS } from './layoutNavigation'

type LayoutSearchState = ReturnType<typeof useLayoutSearch>
type LayoutProfileState = ReturnType<typeof useLayoutProfiles>
type LayoutThemeState = ReturnType<typeof useThemeMode>

type LayoutPermissionState = {
  can: (key: string) => boolean
  isAdmin: boolean
}

type LayoutHeaderProps = {
  search: LayoutSearchState
  permissions: LayoutPermissionState
  theme: LayoutThemeState
  onOpenMobileDrawer: () => void
  user: User | null | undefined
  activeProfileId: string | null
  profile: LayoutProfileState
  onLogout: () => void
}

export function LayoutHeader({
  search,
  permissions,
  theme,
  onOpenMobileDrawer,
  user,
  activeProfileId,
  profile,
  onLogout,
}: LayoutHeaderProps) {
  const { pathname } = useLocation()
  const currentPage = LAYOUT_NAV_GROUPS.flatMap((group) => group.items)
    .find((item) => item.to === '/' ? pathname === '/' : pathname === item.to || pathname.startsWith(`${item.to}/`))?.label
  return (
    <header className="shell-header">
      <div className="shell-breadcrumb"><span>私人影院</span><i>/</i><strong>{currentPage ?? '观影空间'}</strong></div>
      <LayoutHeaderSearch search={search} onOpenMobileDrawer={onOpenMobileDrawer} />
      <LayoutHeaderActions
        permissions={permissions}
        themeMode={theme.mode}
        onThemeChange={theme.setMode}
        user={user}
        isProfileOpen={profile.isProfileOpen}
        profiles={profile.profiles}
        activeProfileId={activeProfileId}
        activeProfile={profile.activeProfile}
        onToggleProfile={() => profile.setIsProfileOpen((open) => !open)}
        onCloseProfile={() => profile.setIsProfileOpen(false)}
        onUseDefaultProfile={profile.useDefaultProfile}
        onSwitchProfile={profile.switchProfile}
        onLogout={onLogout}
      />
    </header>
  )
}

function LayoutHeaderSearch({
  search,
  onOpenMobileDrawer,
}: {
  search: LayoutSearchState
  onOpenMobileDrawer: () => void
}) {
  return (
    <div className="shell-header-search">
      <button
        onClick={onOpenMobileDrawer}
        aria-label="打开导航"
        aria-haspopup="dialog"
        aria-controls="station-mobile-navigation"
        className="shell-icon-button shell-mobile-menu"
      >
        <Menu size={18} />
      </button>
      <LayoutSearchBox
        query={search.query}
        focused={search.focused}
        loading={search.loading}
        error={search.error}
        cards={search.cards}
        total={search.total}
        onQueryChange={search.setQuery}
        onClear={search.clear}
        onFocusedChange={search.setFocused}
        onSubmit={search.submit}
      />
    </div>
  )
}

type LayoutHeaderActionsProps = {
  permissions: LayoutPermissionState
  themeMode: ThemeMode
  onThemeChange: (mode: ThemeMode) => void
  user: User | null | undefined
  isProfileOpen: boolean
  profiles: PlayProfile[]
  activeProfileId: string | null
  activeProfile: PlayProfile | null
  onToggleProfile: () => void
  onCloseProfile: () => void
  onUseDefaultProfile: () => void
  onSwitchProfile: (profile: PlayProfile) => void
  onLogout: () => void
}

function LayoutHeaderActions({
  permissions,
  themeMode,
  onThemeChange,
  user,
  isProfileOpen,
  profiles,
  activeProfileId,
  activeProfile,
  onToggleProfile,
  onCloseProfile,
  onUseDefaultProfile,
  onSwitchProfile,
  onLogout,
}: LayoutHeaderActionsProps) {
  return (
    <div className="shell-header-actions">
      <LayoutQuickActions permissions={permissions} />
      <LayoutThemeToggle mode={themeMode} onChange={onThemeChange} />
      <span className="shell-header-divider" />
      <LayoutProfileMenu
        user={user}
        isProfileOpen={isProfileOpen}
        profiles={profiles}
        activeProfileId={activeProfileId}
        activeProfile={activeProfile}
        onToggleProfile={onToggleProfile}
        onCloseProfile={onCloseProfile}
        onUseDefaultProfile={onUseDefaultProfile}
        onSwitchProfile={onSwitchProfile}
        onLogout={onLogout}
      />
    </div>
  )
}

function LayoutQuickActions({ permissions }: { permissions: LayoutPermissionState }) {
  return (
    <>
      {permissions.can('can_play_media') && <Link
        to="/search"
        className="shell-icon-button shell-mobile-search"
        aria-label="搜索媒体"
      >
        <Search size={18} />
      </Link>}
      {permissions.can('can_view_discover') && (
        <Link
          to="/discover"
          className="shell-discover-link"
        >
          <Compass size={16} />
          <span>发现新片</span><ArrowUpRight size={13} />
        </Link>
      )}
    </>
  )
}

function LayoutProfileMenu({
  user,
  isProfileOpen,
  profiles,
  activeProfileId,
  activeProfile,
  onToggleProfile,
  onCloseProfile,
  onUseDefaultProfile,
  onSwitchProfile,
  onLogout,
}: Omit<LayoutHeaderActionsProps, 'permissions' | 'themeMode' | 'onThemeChange'>) {
  return (
    <LayoutUserMenu
      user={user}
      isOpen={isProfileOpen}
      profiles={profiles}
      activeProfileId={activeProfileId}
      activeProfile={activeProfile}
      onToggle={onToggleProfile}
      onClose={onCloseProfile}
      onUseDefaultProfile={onUseDefaultProfile}
      onSwitchProfile={onSwitchProfile}
      onLogout={onLogout}
    />
  )
}
