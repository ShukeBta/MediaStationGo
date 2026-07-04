import { useEffect, useMemo, useState } from 'react'

import { libraryAPI, mediaAPI } from '../api/library'
import { playbackAPI, type HistoryItem } from '../api/playback'
import type { Library, Media } from '../types'
import { groupSeries, type SeriesCard } from '../utils/groupSeries'
import {
  ContinueWatchingSection,
  HomeEmptyState,
  HomeFeaturedSection,
  HomeLoadingState,
  RecentMediaSection,
} from './HomePageSections'

const hasArtwork = (media?: Media | null) => !!(media?.poster_url || media?.backdrop_url)
const asArray = <T,>(value: unknown): T[] => (Array.isArray(value) ? value as T[] : [])

export function HomePage() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [recentCards, setRecentCards] = useState<SeriesCard[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const [libs, recentItems, hist] = await Promise.all([
          libraryAPI.list().then((rows) => asArray<Library>(rows)).catch(() => [] as Library[]),
          mediaAPI.recent(24).then((rows) => asArray<SeriesCard>(rows)).catch(async () => {
            const fallback = await mediaAPI.search('', 120).then((d) => asArray<Media>(d?.items)).catch(() => [] as Media[])
            return groupSeries(fallback).slice(0, 24)
          }),
          playbackAPI.recentHistory().then((rows) => asArray<HistoryItem>(rows)).catch(() => [] as HistoryItem[]),
        ])
        if (cancelled) return
        setLibraries(libs)
        setRecentCards(recentItems)
        setHistory(hist.filter((h) => h && !h.completed && !!h.media))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [])

  const featuredItem = useMemo(() => {
    const candidates = [
      ...(history.map((h) => h.media).filter(Boolean) as Media[]),
      ...recentCards.map((card) => card.rep),
    ]
    return candidates.find(hasArtwork) ?? candidates[0] ?? null
  }, [history, recentCards])
  const featuredVisual = featuredItem?.backdrop_url || featuredItem?.poster_url || ''
  const featuredPoster = featuredItem?.poster_url || featuredItem?.backdrop_url || ''
  const featuredMark = (featuredItem?.title || 'MS').trim().slice(0, 4).toUpperCase()
  const empty = !loading && libraries.length === 0 && recentCards.length === 0 && history.length === 0

  if (loading) {
    return <HomeLoadingState />
  }

  if (empty) {
    return <HomeEmptyState />
  }

  return (
    <div className="space-y-12">
      {featuredItem && (
        <HomeFeaturedSection
          featuredItem={featuredItem}
          featuredVisual={featuredVisual}
          featuredPoster={featuredPoster}
          featuredMark={featuredMark}
        />
      )}

      {history.length > 0 && <ContinueWatchingSection history={history} />}
      {recentCards.length > 0 && <RecentMediaSection recentCards={recentCards} />}
    </div>
  )
}
