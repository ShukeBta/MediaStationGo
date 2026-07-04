import { useLocation, useNavigate } from 'react-router-dom'

import { useAuthStore } from '../stores/auth'
import { usePlayProfileStore } from '../stores/playProfile'
import {
  LayoutFrameFooter,
  LayoutHeader,
  LayoutSidebars,
  LayoutWorkspace,
} from './LayoutSections'
import { useLayoutSearch } from './useLayoutSearch'
import { useLayoutPermissions } from './useLayoutPermissions'
import { useLayoutProfiles } from './useLayoutProfiles'
import { useLayoutSidebar } from './useLayoutSidebar'
import { useThemeMode } from './useThemeMode'

export function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const activeProfileId = usePlayProfileStore((s) => s.activeProfileId)
  const setActiveProfile = usePlayProfileStore((s) => s.setActiveProfile)
  const theme = useThemeMode()
  const search = useLayoutSearch({
    pathname: location.pathname,
    locationSearch: location.search,
    navigate,
  })
  const permissions = useLayoutPermissions(user)
  const sidebar = useLayoutSidebar(location.pathname)
  const profile = useLayoutProfiles({ activeProfileId, setActiveProfile, user })

  const handleLogout = () => { logout(); navigate('/login') }
  const closeProfileAndLogout = () => { profile.setIsProfileOpen(false); handleLogout() }

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[var(--app-bg)] text-[var(--app-text)] font-body select-none">
      <LayoutSidebars
        sidebar={sidebar}
        isAdmin={permissions.isAdmin}
        username={user?.username}
        can={permissions.can}
        onLogout={handleLogout}
      />
      <div className="flex flex-1 flex-col min-w-0 overflow-hidden">
        <LayoutHeader
          search={search}
          permissions={permissions}
          theme={theme}
          onOpenMobileDrawer={() => sidebar.setIsMobileDrawerOpen(true)}
          user={user}
          activeProfileId={activeProfileId}
          profile={profile}
          onLogout={closeProfileAndLogout}
        />
        <LayoutWorkspace routeKey={location.pathname} />
        <LayoutFrameFooter />
      </div>
    </div>
  )
}
