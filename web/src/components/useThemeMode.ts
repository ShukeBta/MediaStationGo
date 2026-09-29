import { useCallback, useEffect, useState } from 'react'

export type ThemeMode = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const THEME_STORAGE_KEY = 'mediastationgo.theme'
const DARK_QUERY = '(prefers-color-scheme: dark)'

function readStoredTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'light'
  const value = window.localStorage.getItem(THEME_STORAGE_KEY)
  return value === 'light' || value === 'dark' || value === 'system' ? value : 'light'
}

function systemTheme(): ResolvedTheme {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light'
}

function resolveTheme(mode: ThemeMode): ResolvedTheme {
  return mode === 'system' ? systemTheme() : mode
}

function applyTheme(mode: ThemeMode) {
  if (typeof document === 'undefined') return
  const resolved = resolveTheme(mode)
  const root = document.documentElement
  root.dataset.themeMode = mode
  root.dataset.theme = resolved
  root.style.colorScheme = resolved
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', resolved === 'dark' ? '#1c2430' : '#f7f8fa')
}

export function initializeThemeMode() {
  applyTheme(readStoredTheme())
}

export function useThemeMode() {
  const [mode, setModeState] = useState<ThemeMode>(() => readStoredTheme())
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() => resolveTheme(readStoredTheme()))

  useEffect(() => {
    applyTheme(mode)
    setResolvedTheme(resolveTheme(mode))
    window.localStorage.setItem(THEME_STORAGE_KEY, mode)
  }, [mode])

  useEffect(() => {
    const media = window.matchMedia(DARK_QUERY)
    const update = () => {
      if (mode === 'system') {
        applyTheme(mode)
        setResolvedTheme(resolveTheme(mode))
      }
    }
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [mode])

  const setMode = useCallback((nextMode: ThemeMode) => {
    setModeState(nextMode)
  }, [])

  return { mode, resolvedTheme, setMode }
}
