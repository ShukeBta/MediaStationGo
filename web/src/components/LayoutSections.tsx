import { Outlet } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import clsx from 'clsx'

import { AppFooter } from './AppFooter'
import { LayoutSidebarContent, type LayoutSidebarContentProps } from './LayoutSidebarContent'
import { RouteErrorBoundary } from './RouteErrorBoundary'
import type { useLayoutSidebar } from './useLayoutSidebar'

type LayoutSidebarState = ReturnType<typeof useLayoutSidebar>

type LayoutSidebarProps = {
  children: React.ReactNode
  isSidebarOpen: boolean
}

type LayoutMobileSidebarProps = {
  children: React.ReactNode
  isOpen: boolean
  onClose: () => void
}

type LayoutSidebarsProps = Omit<
  LayoutSidebarContentProps,
  'isSidebarOpen' | 'isMobileDrawerOpen' | 'openGroups' | 'isRouteIn' | 'onToggleGroup' | 'onToggleSidebar' | 'onCloseMobileDrawer'
> & {
  sidebar: LayoutSidebarState
}

type LayoutWorkspaceProps = {
  routeKey: string
}

export { LayoutHeader } from './LayoutHeaderSections'

export function LayoutDesktopSidebar({ children, isSidebarOpen }: LayoutSidebarProps) {
  return (
    <aside
      className={clsx(
        'hidden lg:flex flex-col h-full shrink-0 transition-all duration-300 ease-out',
        isSidebarOpen ? 'w-64' : 'w-20',
      )}
    >
      {children}
    </aside>
  )
}

export function LayoutMobileSidebar({ children, isOpen, onClose }: LayoutMobileSidebarProps) {
  return (
    <AnimatePresence>
      {isOpen && (
        <div className="fixed inset-0 z-50 flex lg:hidden">
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="fixed inset-0 bg-black/15 backdrop-blur-sm"
          />
          <motion.div
            initial={{ x: '-100%' }}
            animate={{ x: 0 }}
            exit={{ x: '-100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 220 }}
            className="relative flex w-64 max-w-xs flex-col h-full z-10 shadow-xl"
          >
            {children}
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}

export function LayoutSidebars({
  sidebar,
  isAdmin,
  username,
  can,
  onLogout,
}: LayoutSidebarsProps) {
  const content = (
    <LayoutSidebarContent
      isSidebarOpen={sidebar.isSidebarOpen}
      isMobileDrawerOpen={sidebar.isMobileDrawerOpen}
      openGroups={sidebar.openGroups}
      isAdmin={isAdmin}
      username={username}
      can={can}
      isRouteIn={sidebar.isRouteIn}
      onToggleGroup={sidebar.toggleGroup}
      onToggleSidebar={() => sidebar.setIsSidebarOpen((current) => !current)}
      onCloseMobileDrawer={() => sidebar.setIsMobileDrawerOpen(false)}
      onLogout={onLogout}
    />
  )

  return (
    <>
      <LayoutDesktopSidebar isSidebarOpen={sidebar.isSidebarOpen}>{content}</LayoutDesktopSidebar>
      <LayoutMobileSidebar
        isOpen={sidebar.isMobileDrawerOpen}
        onClose={() => sidebar.setIsMobileDrawerOpen(false)}
      >
        {content}
      </LayoutMobileSidebar>
    </>
  )
}

export function LayoutWorkspace({ routeKey }: LayoutWorkspaceProps) {
  return (
    <main className="flex-1 overflow-y-auto px-4 py-6 md:px-8 md:py-10">
      <div className="max-w-7xl mx-auto">
        <AnimatePresence mode="wait">
          <motion.div
            key={routeKey}
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.25, ease: 'easeOut' }}
          >
            <RouteErrorBoundary>
              <Outlet />
            </RouteErrorBoundary>
          </motion.div>
        </AnimatePresence>
      </div>
    </main>
  )
}

export function LayoutFrameFooter() {
  return (
    <AppFooter className="border-t border-[var(--app-border)] bg-[var(--app-panel)] py-5 text-center text-xs text-[var(--app-muted)]" />
  )
}

export { LayoutSidebarContent }
