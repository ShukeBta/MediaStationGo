import { useEffect, useMemo } from 'react'

import type { Media } from '../types'
import { getSeriesKey, type SeriesCard } from '../utils/groupSeries'

type SeasonEpisodes = {
  season: number
  episodes: Media[]
}

type UseLibrarySeriesSelectionOptions = {
  items: Media[]
  seriesEpisodeItems: Media[]
  isSeriesLibrary: boolean
  isSeries: boolean
  loading: boolean
  seriesCards: SeriesCard[]
  searchParams: URLSearchParams
  setSearchParams: (params: URLSearchParams) => void
  selectedSeries: SeriesCard | null
  setSelectedSeries: (series: SeriesCard | null) => void
  selectedSeason: number | null
  setSelectedSeason: (season: number | null) => void
  onClearSeriesState?: () => void
}

export function useLibrarySeriesSelection({
  items,
  seriesEpisodeItems,
  isSeriesLibrary,
  isSeries,
  loading,
  seriesCards,
  searchParams,
  setSearchParams,
  selectedSeries,
  setSelectedSeries,
  selectedSeason,
  setSelectedSeason,
  onClearSeriesState,
}: UseLibrarySeriesSelectionOptions) {
  const selectedEpisodes = useMemo(() => {
    const sourceItems = isSeriesLibrary ? seriesEpisodeItems : items
    if (!selectedSeries || sourceItems.length === 0) return []
    const eps = isSeriesLibrary
      ? sourceItems
      : sourceItems.filter((m) => getSeriesKey(m) === selectedSeries.key)
    const seasons = new Map<number, Media[]>()
    for (const ep of eps) {
      const s = ep.episode_num > 0 ? (ep.season_num ?? 0) : (ep.season_num || 1)
      if (!seasons.has(s)) seasons.set(s, [])
      seasons.get(s)!.push(ep)
    }
    for (const [, list] of seasons) {
      list.sort((a, b) => (a.episode_num || 0) - (b.episode_num || 0))
    }
    return Array.from(seasons.entries())
      .sort(([a], [b]) => a - b)
      .map(([season, episodes]) => ({ season, episodes }))
  }, [isSeriesLibrary, selectedSeries, items, seriesEpisodeItems])

  const visibleEpisodes = useMemo(() => {
    if (selectedSeason == null) return selectedEpisodes[0]?.episodes ?? []
    return selectedEpisodes.find((s) => s.season === selectedSeason)?.episodes ?? []
  }, [selectedEpisodes, selectedSeason])

  const selectedSeriesEpisodes = useMemo(
    () => selectedEpisodes.flatMap((season: SeasonEpisodes) => season.episodes),
    [selectedEpisodes],
  )

  const selectedSeriesMediaIDs = useMemo(
    () => selectedSeriesEpisodes.map((ep) => ep.id),
    [selectedSeriesEpisodes],
  )

  useEffect(() => {
    if (loading) return
    if (!isSeries) {
      setSelectedSeries(null)
      setSelectedSeason(null)
      return
    }

    const key = searchParams.get('series')
    if (!key) {
      setSelectedSeries(null)
      return
    }

    const next = seriesCards.find((card) => card.key === key)
    setSelectedSeries(next ?? null)
  }, [isSeries, loading, searchParams, seriesCards, setSelectedSeason, setSelectedSeries])

  useEffect(() => {
    if (!selectedSeries || selectedEpisodes.length === 0) {
      setSelectedSeason(null)
      return
    }
    if (selectedSeason == null || !selectedEpisodes.some((s) => s.season === selectedSeason)) {
      setSelectedSeason(selectedEpisodes[0].season)
    }
  }, [selectedSeries, selectedEpisodes, selectedSeason, setSelectedSeason])

  const handleSeriesClick = (card: SeriesCard) => {
    setSelectedSeries(card)
    const next = new URLSearchParams(searchParams)
    next.set('series', card.key)
    setSearchParams(next)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const clearSelectedSeries = () => {
    setSelectedSeries(null)
    setSelectedSeason(null)
    onClearSeriesState?.()
    const next = new URLSearchParams(searchParams)
    next.delete('series')
    setSearchParams(next)
  }

  return {
    selectedEpisodes,
    visibleEpisodes,
    selectedSeriesEpisodes,
    selectedSeriesMediaIDs,
    handleSeriesClick,
    clearSelectedSeries,
  }
}
