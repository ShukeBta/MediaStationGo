import { useEffect, useRef, useState } from 'react'

import { libraryAPI, mediaAPI } from '../api/library'
import { playbackAPI, type HistoryItem } from '../api/playback'
import { usePermission } from '../hooks/usePermission'
import type { Library } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { useAuthStore } from '../stores/auth'
import { HomeHero } from './HomeHero'
import { loadRandomHero } from './homePageModel'
import '../styles/home.css'
import {
  ContinueWatchingSection,
  HomeEmptyState,
  HomeLibraryShortcuts,
  HomeWelcome,
  HomeLoadError,
  HomeLoadingState,
  RecentMediaSection,
} from './HomePageSections'

const asArray = <T,>(value: unknown): T[] => (Array.isArray(value) ? value as T[] : [])

export function HomePage() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [featuredCard, setFeaturedCard] = useState<SeriesCard | null>(null)
  const featuredPage = useRef(0)
  const shuffleRequest = useRef(0)
  const [shuffling, setShuffling] = useState(false)
  const [shuffleError, setShuffleError] = useState('')
  const [recentCards, setRecentCards] = useState<SeriesCard[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [loadVersion, setLoadVersion] = useState(0)
  const canPlayMedia = usePermission('can_play_media')
  const canViewHistory = usePermission('can_view_history')
  const canDiscover = usePermission('can_view_discover')
  const user = useAuthStore((state) => state.user)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError('')
      setShuffling(false)
      setShuffleError('')
      try {
        const [libs, featured, recentItems, hist] = await Promise.all([
          canPlayMedia ? libraryAPI.list().then((rows) => asArray<Library>(rows)) : Promise.resolve([] as Library[]),
          canPlayMedia ? loadRandomHero((page) => mediaAPI.searchSeriesPage('', page, 1)) : Promise.resolve({ item: null, page: 0 }),
          mediaAPI.recent(24).then((rows) => asArray<SeriesCard>(rows)),
          canViewHistory ? playbackAPI.recentHistory().then((rows) => asArray<HistoryItem>(rows)) : Promise.resolve([] as HistoryItem[]),
        ])
        if (cancelled) return
        setLibraries(libs)
        setFeaturedCard(featured.item ?? null)
        featuredPage.current = featured.page
        setRecentCards(recentItems)
        setHistory(hist.filter((h) => h && !h.completed && !!h.media))
      } catch (err) {
        if (cancelled) return
        setLibraries([])
        setFeaturedCard(null)
        setRecentCards([])
        setHistory([])
        setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error || '首页内容加载失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true; shuffleRequest.current += 1 }
  }, [canPlayMedia, canViewHistory, loadVersion])

  async function shuffleHero() {
    if (shuffling) return
    const request = ++shuffleRequest.current
    setShuffling(true)
    setShuffleError('')
    try {
      const selected = await loadRandomHero((page) => mediaAPI.searchSeriesPage('', page, 1), featuredPage.current)
      if (request !== shuffleRequest.current) return
      setFeaturedCard(selected.item)
      featuredPage.current = selected.page
    } catch {
      if (request === shuffleRequest.current) setShuffleError('暂时无法换片，请稍后重试')
    } finally {
      if (request === shuffleRequest.current) setShuffling(false)
    }
  }

  if (loading) {
    return <HomeLoadingState />
  }

  if (error) {
    return <HomeLoadError message={error} onRetry={() => setLoadVersion((version) => version + 1)} />
  }

  return (
    <div className="cinema-home">
      <HomeWelcome name={user?.nickname || user?.username || '影迷'} libraryCount={libraries.length} canPlay={canPlayMedia} />
      <HomeHero card={featuredCard} canPlay={canPlayMedia} canDiscover={canDiscover} onShuffle={shuffleHero} shuffling={shuffling} shuffleError={shuffleError} />
      {canPlayMedia && <HomeLibraryShortcuts libraries={libraries} />}
      {history.length > 0 && <ContinueWatchingSection history={history} />}
      {recentCards.length > 0 && <RecentMediaSection recentCards={recentCards} libraries={libraries} />}
      {recentCards.length === 0 && history.length === 0 && <HomeEmptyState canManage={user?.role === 'admin'} hasLibraries={libraries.length > 0} />}
    </div>
  )
}
