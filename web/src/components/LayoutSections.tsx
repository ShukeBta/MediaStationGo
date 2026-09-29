import { useEffect, useRef } from 'react'
import { Outlet } from 'react-router-dom'
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
import clsx from 'clsx'

import { AppFooter } from './AppFooter'
import { LayoutSidebarContent, type LayoutSidebarContentProps } from './LayoutSidebarContent'
import { RouteErrorBoundary } from './RouteErrorBoundary'
import type { useLayoutSidebar } from './useLayoutSidebar'

type LayoutSidebarState = ReturnType<typeof useLayoutSidebar>
type LayoutSidebarProps = { children: React.ReactNode; isSidebarOpen: boolean }
type LayoutMobileSidebarProps = { children: React.ReactNode; isOpen: boolean; onClose: () => void }
type LayoutSidebarsProps = Omit<
  LayoutSidebarContentProps,
  'isSidebarOpen' | 'isMobileDrawerOpen' | 'openGroups' | 'isRouteIn' | 'onToggleGroup' | 'onToggleSidebar' | 'onCloseMobileDrawer'
> & { sidebar: LayoutSidebarState }

export { LayoutHeader } from './LayoutHeaderSections'

export function LayoutDesktopSidebar({ children, isSidebarOpen }: LayoutSidebarProps) {
  return (
    <aside data-shell-background className={clsx('shell-desktop-sidebar', !isSidebarOpen && 'is-collapsed')}>
      {children}
    </aside>
  )
}

export function LayoutMobileSidebar({ children, isOpen, onClose }: LayoutMobileSidebarProps) {
  const drawerRef = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  const reduceMotion = useReducedMotion()
  useEffect(() => { onCloseRef.current = onClose }, [onClose])

  useEffect(() => {
    if (!isOpen) return
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const backgrounds = Array.from(document.querySelectorAll<HTMLElement>('[data-shell-background]'))
    const previousStates = backgrounds.map((element) => ({ element, inert: element.inert, hidden: element.getAttribute('aria-hidden') }))
    backgrounds.forEach((element) => { element.inert = true; element.setAttribute('aria-hidden', 'true') })
    const drawer = drawerRef.current
    const focusable = () => Array.from(drawer?.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), [tabindex="0"]') ?? [])
      .filter((element) => element.getClientRects().length > 0)
    ;(focusable()[0] ?? drawer)?.focus()

    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onCloseRef.current(); return }
      if (event.key !== 'Tab') return
      const items = focusable()
      const first = items[0]
      const last = items[items.length - 1]
      if (!first) { event.preventDefault(); drawer?.focus(); return }
      if (event.shiftKey && (document.activeElement === first || document.activeElement === drawer || !drawer?.contains(document.activeElement))) {
        event.preventDefault(); last.focus()
      } else if (!event.shiftKey && (document.activeElement === last || !drawer?.contains(document.activeElement))) {
        event.preventDefault(); first.focus()
      }
    }
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('keydown', handleKey)
      previousStates.forEach(({ element, inert, hidden }) => {
        element.inert = inert
        if (hidden === null) element.removeAttribute('aria-hidden')
        else element.setAttribute('aria-hidden', hidden)
      })
      if (previousFocus?.isConnected) previousFocus.focus()
    }
  }, [isOpen])

  return (
    <AnimatePresence>
      {isOpen && (
        <div className="shell-mobile-layer">
          <motion.div aria-hidden="true" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={onClose} className="shell-drawer-backdrop" />
          <motion.div
            ref={drawerRef} id="station-mobile-navigation" role="dialog" aria-modal="true" aria-label="主导航" tabIndex={-1}
            initial={{ x: reduceMotion ? 0 : '-100%', opacity: reduceMotion ? 0 : 1 }}
            animate={{ x: 0, opacity: 1 }} exit={{ x: reduceMotion ? 0 : '-100%', opacity: reduceMotion ? 0 : 1 }}
            transition={{ type: 'spring', damping: 32, stiffness: 310 }} className="shell-mobile-drawer"
          >{children}</motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}

export function LayoutSidebars({ sidebar, isAdmin, username, can, onLogout }: LayoutSidebarsProps) {
  const content = (
    <LayoutSidebarContent
      isSidebarOpen={sidebar.isSidebarOpen} isMobileDrawerOpen={sidebar.isMobileDrawerOpen}
      openGroups={sidebar.openGroups} isAdmin={isAdmin} username={username} can={can} isRouteIn={sidebar.isRouteIn}
      onToggleGroup={sidebar.toggleGroup} onToggleSidebar={() => sidebar.setIsSidebarOpen((current) => !current)}
      onCloseMobileDrawer={() => sidebar.setIsMobileDrawerOpen(false)} onLogout={onLogout}
    />
  )
  return <>
    <LayoutDesktopSidebar isSidebarOpen={sidebar.isSidebarOpen}>{content}</LayoutDesktopSidebar>
    <LayoutMobileSidebar isOpen={sidebar.isMobileDrawerOpen} onClose={() => sidebar.setIsMobileDrawerOpen(false)}>{content}</LayoutMobileSidebar>
  </>
}

export function LayoutWorkspace({ routeKey }: { routeKey: string }) {
  const reduceMotion = useReducedMotion()
  return (
    <main id="main-content" tabIndex={-1} className="shell-workspace">
      <div className={clsx('shell-page', routeKey === '/discover' && 'shell-page-wide')}>
        <AnimatePresence mode="wait">
          <motion.div key={routeKey}
            initial={{ opacity: 0, y: reduceMotion ? 0 : 20 }} animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: reduceMotion ? 0 : -6 }} transition={{ duration: reduceMotion ? 0.08 : 0.38, ease: [.22, 1, .36, 1] }}
          ><RouteErrorBoundary><Outlet /></RouteErrorBoundary></motion.div>
        </AnimatePresence>
      </div>
      <LayoutFrameFooter />
    </main>
  )
}

export function LayoutFrameFooter() {
  return <AppFooter className="shell-footer" />
}

export { LayoutSidebarContent }
