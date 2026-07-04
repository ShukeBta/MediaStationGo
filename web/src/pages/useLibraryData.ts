import { useCallback, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import type { Library, Media } from '../types'
import { groupSeries, isEpisodeLike, type SeriesCard } from '../utils/groupSeries'

export function useLibraryData(libraryID: string, selectedSeries: SeriesCard | null) {
  const [library, setLibrary] = useState<Library | null>(null)
  const [items, setItems] = useState<Media[]>([])
  const [serverSeriesCards, setServerSeriesCards] = useState<SeriesCard[]>([])
  const [seriesEpisodeItems, setSeriesEpisodeItems] = useState<Media[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingAll, setLoadingAll] = useState(false)
  const [loadingSeriesEpisodes, setLoadingSeriesEpisodes] = useState(false)

  const isSeriesLibrary = isSeriesLibraryType(library?.type)
  const hasEpisodicItems = useMemo(() => items.some(isEpisodeLike), [items])
  const isSeries = isSeriesLibrary || serverSeriesCards.length > 0 || hasEpisodicItems

  const seriesCards = useMemo(() => {
    if (isSeriesLibrary) return serverSeriesCards
    if (!isSeries || items.length === 0) return []
    return groupSeries(items)
  }, [isSeries, isSeriesLibrary, items, serverSeriesCards])

  useEffect(() => {
    if (!libraryID) return
    let cancelled = false
    setLoading(true)
    setLibrary(null)
    setItems([])
    setServerSeriesCards([])
    setSeriesEpisodeItems([])
    libraryAPI.get(libraryID)
      .then((lib) => {
        if (!cancelled) setLibrary(lib)
      })
      .catch(() => {
        if (!cancelled) {
          setLibrary(null)
          setLoading(false)
          toast.error('媒体库不存在或无权限')
        }
      })
    return () => { cancelled = true }
  }, [libraryID])

  useEffect(() => {
    if (!libraryID || !library) return
    let cancelled = false
    setLoading(true)
    setLoadingAll(true)
    setItems([])
    setServerSeriesCards([])
    setSeriesEpisodeItems([])

    const loadAll = async () => {
      if (isSeriesLibrary) {
        const collected = await loadAllSeriesCards(libraryID, (next) => {
          if (cancelled) return
          setTotal(next.total)
          if (next.firstPage) {
            setServerSeriesCards(next.items)
            setLoading(false)
          }
        })
        if (!cancelled) setServerSeriesCards(collected.items)
        return
      }

      const collected = await loadAllMedia(libraryID, (next) => {
        if (cancelled) return
        setTotal(next.total)
        if (next.firstPage) {
          setItems(next.items)
          setLoading(false)
        }
      })
      if (!cancelled) setItems(collected.items)
    }

    loadAll()
      .catch(() => {
        if (!cancelled) toast.error('媒体库加载失败')
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
          setLoadingAll(false)
        }
      })
    return () => { cancelled = true }
  }, [libraryID, library, isSeriesLibrary])

  useEffect(() => {
    if (!libraryID || !isSeriesLibrary || !selectedSeries) {
      setSeriesEpisodeItems([])
      setLoadingSeriesEpisodes(false)
      return
    }
    let cancelled = false
    setLoadingSeriesEpisodes(true)
    setSeriesEpisodeItems([])
    libraryAPI.listSeriesEpisodes(libraryID, selectedSeries.key)
      .then((r) => {
        if (!cancelled) setSeriesEpisodeItems(r.items ?? [])
      })
      .catch(() => {
        if (!cancelled) toast.error('剧集列表加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoadingSeriesEpisodes(false)
      })
    return () => { cancelled = true }
  }, [libraryID, isSeriesLibrary, selectedSeries])

  const reloadCurrentLibrary = useCallback(() => {
    setLibrary((current) => (current ? { ...current } : current))
  }, [])

  const loadingAllText = loadingAll && !loading && (isSeriesLibrary ? total > serverSeriesCards.length : total > items.length)
    ? (isSeriesLibrary
      ? `正在继续加载剧集卡片：${serverSeriesCards.length} / ${total}`
      : `正在继续加载全部条目：${items.length} / ${total}`)
    : ''

  return {
    library,
    items,
    seriesEpisodeItems,
    total,
    loading,
    loadingSeriesEpisodes,
    isSeriesLibrary,
    isSeries,
    seriesCards,
    loadingAllText,
    reloadCurrentLibrary,
  }
}

function isSeriesLibraryType(type?: string) {
  return type === 'tv' || type === 'anime' || type === 'variety'
}

async function loadAllSeriesCards(
  libraryID: string,
  onPage: (state: { items: SeriesCard[]; total: number; firstPage: boolean }) => void,
) {
  const pageSize = 500
  let page = 1
  let collected: SeriesCard[] = []
  for (;;) {
    const data = await libraryAPI.listSeries(libraryID, page, pageSize)
    // 后端对空库可能返回 items: null（Go nil slice）；不兜底会 concat 出 [null] 并崩溃。
    const pageItems = data.items ?? []
    collected = collected.concat(pageItems)
    onPage({ items: collected, total: data.total ?? collected.length, firstPage: page === 1 })
    if (collected.length >= (data.total ?? 0) || pageItems.length < pageSize) break
    page += 1
  }
  return { items: collected }
}

async function loadAllMedia(
  libraryID: string,
  onPage: (state: { items: Media[]; total: number; firstPage: boolean }) => void,
) {
  const pageSize = 2000
  let page = 1
  let collected: Media[] = []
  for (;;) {
    const data = await libraryAPI.listMedia(libraryID, page, pageSize)
    // 后端对空库可能返回 items: null（Go nil slice）；不兜底会 concat 出 [null] 并崩溃。
    const pageItems = data.items ?? []
    collected = collected.concat(pageItems)
    onPage({ items: collected, total: data.total ?? collected.length, firstPage: page === 1 })
    if (collected.length >= (data.total ?? 0) || pageItems.length < pageSize) break
    page += 1
  }
  return { items: collected }
}
