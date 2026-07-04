import { useCallback, useEffect, useState } from 'react'

import { NAV_GROUP_PATHS } from './layoutNavigation'

export function useLayoutSidebar(pathname: string) {
  const [isSidebarOpen, setIsSidebarOpen] = useState(true)
  const [isMobileDrawerOpen, setIsMobileDrawerOpen] = useState(false)
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({ media: true })

  useEffect(() => {
    const handleResize = () => {
      setIsSidebarOpen(window.innerWidth >= 1024)
    }
    handleResize()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  useEffect(() => {
    setIsMobileDrawerOpen(false)
  }, [pathname])

  const isRouteIn = useCallback(
    (paths: string[]) =>
      paths.some((path) => (path === '/' ? pathname === '/' : pathname.startsWith(path))),
    [pathname],
  )

  const toggleGroup = useCallback(
    (key: string) => setOpenGroups((current) => ({ ...current, [key]: !current[key] })),
    [],
  )

  useEffect(() => {
    const active = Object.entries(NAV_GROUP_PATHS).find(([, paths]) => isRouteIn(paths))?.[0]
    if (active) {
      setOpenGroups((current) => (current[active] ? current : { ...current, [active]: true }))
    }
  }, [isRouteIn])

  return {
    isMobileDrawerOpen,
    isRouteIn,
    isSidebarOpen,
    openGroups,
    setIsMobileDrawerOpen,
    setIsSidebarOpen,
    toggleGroup,
  }
}
