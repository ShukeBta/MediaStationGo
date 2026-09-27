import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { libraryAPI, type LibraryBrowseOptions, type LibraryBrowsePage } from '../api/library'
import type { Library, Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'

export function useLibraryData(libraryID: string, selectedSeries: SeriesCard | null, options: LibraryBrowseOptions) {
  const [library, setLibrary] = useState<Library | null>(null)
  const [snapshot, setSnapshot] = useState<{ libraryID: string; key: string; canonicalKey: string; data: LibraryBrowsePage } | null>(null)
  const snapshotRef = useRef(snapshot)
  const [libraryLoadedKey, setLibraryLoadedKey] = useState('')
  const [failure, setFailure] = useState<{ key: string; message: string } | null>(null)
  const [seriesEpisodeItems, setSeriesEpisodeItems] = useState<Media[]>([])
  const [loadingSeriesEpisodes, setLoadingSeriesEpisodes] = useState(false)
  const [reloadVersion, setReloadVersion] = useState(0)
  const facetsRef = useRef<{ scope: string; data: NonNullable<LibraryBrowsePage['facets']> } | null>(null)
  const { page, q = '', sort = '', category = '', genre = '', year = '', language = '', actor = '', adult_type = '', series = '', focus_media = '' } = options
  const requestKey = JSON.stringify([libraryID, page, q, sort, category, genre, year, language, actor, adult_type, series, focus_media, reloadVersion])
  const facetScope = JSON.stringify([libraryID, reloadVersion])
  const libraryKey = `${libraryID}:${reloadVersion}`

  useEffect(() => {
    let cancelled = false
    libraryAPI.get(libraryID).then((lib) => {
      if (!cancelled) {
        setLibrary(lib)
        setLibraryLoadedKey(libraryKey)
      }
    }).catch(() => {
      if (!cancelled) setFailure({ key: libraryKey, message: '媒体库不存在、无权限或加载失败' })
    })
    return () => { cancelled = true }
  }, [libraryID, libraryKey])

  useEffect(() => {
    if (!library || library.id !== libraryID || libraryLoadedKey !== libraryKey) return
    if (snapshotRef.current?.key === requestKey || snapshotRef.current?.canonicalKey === requestKey) return
    const controller = new AbortController()
    libraryAPI.browse(libraryID, {
      page, q, sort, category, genre, year: year || undefined, language, series, focus_media,
      actor, adult_type,
      // The card page is the visible part of opening a library. Facets inspect
      // every logical card, so load them independently below instead of making
      // the first render wait for a full-library aggregation.
      facets: 0,
    }, controller.signal).then((data) => {
      if (controller.signal.aborted) return
      if (data.facets) facetsRef.current = { scope: facetScope, data: data.facets }
      const next = {
        libraryID, key: requestKey,
        canonicalKey: JSON.stringify([libraryID, data.page, q, sort, category, genre, year, language, actor, adult_type, series, '', reloadVersion]),
        data: { ...data, facets: data.facets ?? (facetsRef.current?.scope === facetScope ? facetsRef.current.data : undefined) },
      }
      snapshotRef.current = next
      setSnapshot(next)
      setFailure(null)
    }).catch(() => {
      if (!controller.signal.aborted) setFailure({ key: requestKey, message: '媒体库加载失败，请重试' })
    })
    return () => controller.abort()
  }, [library, libraryID, libraryLoadedKey, libraryKey, page, q, sort, category, genre, year, language, actor, adult_type, series, focus_media, facetScope, requestKey, reloadVersion])

  useEffect(() => {
    const primaryReady = snapshot?.libraryID === libraryID && (snapshot.key === requestKey || snapshot.canonicalKey === requestKey)
    if (!library || library.id !== libraryID || libraryLoadedKey !== libraryKey || !primaryReady || facetsRef.current?.scope === facetScope) return
    const controller = new AbortController()
    // Facets are library-wide by design, so filters and the current page do
    // not affect this background request or its cache identity.
    libraryAPI.browse(libraryID, { page: 1, facets: 1, facet_only: 1 }, controller.signal).then((data) => {
      if (controller.signal.aborted || !data.facets) return
      facetsRef.current = { scope: facetScope, data: data.facets }
      setSnapshot((current) => {
        if (!current || current.libraryID !== libraryID) return current
        return { ...current, data: { ...current.data, facets: data.facets } }
      })
    }).catch(() => {
      // A card page remains usable when only optional filter facets fail.
    })
    return () => controller.abort()
  }, [library, libraryID, libraryLoadedKey, libraryKey, facetScope, requestKey, snapshot])

  const data = snapshot?.libraryID === libraryID ? snapshot.data : null
  const isSeries = data?.is_series ?? ['tv', 'anime', 'variety'].includes(library?.type ?? '')
  const selectedKey = selectedSeries?.key ?? ''
  useEffect(() => {
    setSeriesEpisodeItems([])
    if (!libraryID || !isSeries || !selectedKey) {
      setLoadingSeriesEpisodes(false)
      return
    }
    const controller = new AbortController()
    setLoadingSeriesEpisodes(true)
    libraryAPI.listSeriesEpisodes(libraryID, selectedKey, controller.signal).then((r) => {
      if (!controller.signal.aborted) setSeriesEpisodeItems(r.items ?? [])
    }).catch(() => {
      if (!controller.signal.aborted) toast.error('剧集列表加载失败')
    }).finally(() => {
      if (!controller.signal.aborted) setLoadingSeriesEpisodes(false)
    })
    return () => controller.abort()
  }, [libraryID, isSeries, selectedKey, reloadVersion])

  const reloadCurrentLibrary = useCallback(() => setReloadVersion((value) => value + 1), [])
  const error = failure?.key === requestKey || failure?.key === libraryKey ? failure.message : ''
  return {
    library: library?.id === libraryID ? library : null,
    items: data?.items ?? [], seriesCards: data?.series_cards ?? [], linkedSeries: data?.selected_series ?? null,
    seriesEpisodeItems, total: data?.total ?? 0, page: data?.page ?? page,
    loading: !data && !error, loadingPage: snapshot?.key !== requestKey && snapshot?.canonicalKey !== requestKey && !error, error,
    loadingSeriesEpisodes, isSeriesLibrary: isSeries, isSeries,
    facets: data?.facets ?? null,
    focusedMediaID: data?.focused_media_id ?? '',
    reloadCurrentLibrary,
  }
}
