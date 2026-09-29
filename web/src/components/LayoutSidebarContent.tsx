import { Link } from 'react-router-dom'
import { LogOut, PanelLeftClose, PanelLeftOpen, Play, X } from 'lucide-react'
import clsx from 'clsx'
import { LAYOUT_NAV_GROUPS, NAV_GROUP_PATHS, type LayoutNavGroup, type LayoutNavItem } from './layoutNavigation'
import { SidebarGroup, SidebarLink } from './LayoutSidebarNav'

export type LayoutSidebarContentProps = {
  isSidebarOpen: boolean
  isMobileDrawerOpen: boolean
  openGroups: Record<string, boolean>
  isAdmin: boolean
  username?: string
  can: (key: string) => boolean
  isRouteIn: (paths: string[]) => boolean
  onToggleGroup: (id: string) => void
  onToggleSidebar: () => void
  onCloseMobileDrawer: () => void
  onLogout: () => void
}

type VisibleLayoutNavGroup = { group: LayoutNavGroup; items: LayoutNavItem[] }

export function LayoutSidebarContent({
  isSidebarOpen, isMobileDrawerOpen, openGroups, isAdmin, username, can, isRouteIn,
  onToggleGroup, onToggleSidebar, onCloseMobileDrawer, onLogout,
}: LayoutSidebarContentProps) {
  const expanded = isSidebarOpen || isMobileDrawerOpen
  const isItemVisible = (item: LayoutNavItem) =>
    (!item.adminOnly || isAdmin) && (!item.permission || can(item.permission))
  const groups: VisibleLayoutNavGroup[] = LAYOUT_NAV_GROUPS
    .filter((group) => !group.adminOnly || isAdmin)
    .map((group) => ({ group, items: group.items.filter(isItemVisible) }))
    .filter(({ items }) => items.length > 0)

  return (
    <div className={clsx('shell-sidebar', !expanded && 'is-collapsed')}>
      <div className="shell-brand-row">
        <Link to="/" className="shell-brand" aria-label="MediaStationGo 首页">
          <span className="shell-brand-mark" aria-hidden="true"><Play size={18} fill="currentColor" strokeWidth={1.5} /></span>
          {expanded && <span className="shell-brand-type"><strong>MediaStation<span>Go</span></strong><small>YOUR PRIVATE CINEMA</small></span>}
        </Link>
        <button type="button" className="shell-icon-button shell-drawer-close" aria-label="关闭导航" onClick={onCloseMobileDrawer}><X size={19} /></button>
      </div>
      <nav className="shell-navigation" aria-label="主导航" onClick={(event) => {
        if (isMobileDrawerOpen && event.target instanceof Element && event.target.closest('a[href]')) onCloseMobileDrawer()
      }}>
        {groups.map(({ group, items }) => {
          const GroupIcon = group.icon
          return <SidebarGroup key={group.id} id={group.id} icon={<GroupIcon size={17} />} label={group.label}
            collapsed={!expanded} open={openGroups[group.id] ?? group.id === 'media'}
            active={isRouteIn(NAV_GROUP_PATHS[group.id])} onToggle={onToggleGroup}>
            {items.map((item) => {
              const Icon = item.icon
              return <SidebarLink key={item.to} to={item.to} icon={<Icon size={18} strokeWidth={1.6} />}
                label={item.label} end={item.end} collapsed={!expanded} />
            })}
          </SidebarGroup>
        })}
      </nav>
      <div className="shell-sidebar-bottom">
        <button type="button" onClick={onToggleSidebar} aria-label={expanded ? '收起侧栏' : '展开侧栏'}
          title={expanded ? '收起侧栏' : '展开侧栏'} aria-expanded={expanded}
          className={clsx('shell-sidebar-action shell-collapse-button', !expanded && 'is-collapsed')}>
          {expanded ? <PanelLeftClose size={17} /> : <PanelLeftOpen size={17} />}
          {expanded && <span>收起侧栏</span>}
        </button>
        <button type="button" onClick={onLogout} title={`安全退出 (${username ?? ''})`} aria-label="安全退出"
          className={clsx('shell-sidebar-action shell-logout', !expanded && 'is-collapsed')}>
          <LogOut size={17} />{expanded && <span>安全退出</span>}
        </button>
        {expanded && <p className="shell-sidebar-signature">MADE FOR YOUR SCREEN.</p>}
      </div>
    </div>
  )
}
