import { FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import toast from 'react-hot-toast'

import { aiAPI, type ExternalMediaResult, type SearchIntent } from '../api/ai'
import { mediaAPI } from '../api/library'
import { discoverAPI } from '../api/discover'
import { groupSeries, type SeriesCard } from '../utils/groupSeries'

const LOCAL_SEARCH_PAGE_SIZE = 36

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

export function useSearchPage({ canUseAI, canViewDiscover = true }: { canUseAI: boolean; canViewDiscover?: boolean }) {
  const [searchParams, setSearchParams] = useSearchParams()
  const urlQuery = searchParams.get('q') ?? ''
  const [q, setQ] = useState('')
  const [localCards, setLocalCards] = useState<SeriesCard[]>([])
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const [aiOn, setAiOn] = useState(false)
  const [aiAvailable, setAiAvailable] = useState(false)
  const [intent, setIntent] = useState<SearchIntent | null>(null)
  const [hasSearched, setHasSearched] = useState(false)
  const [externalItems, setExternalItems] = useState<ExternalMediaResult[]>([])
  const [externalLoading, setExternalLoading] = useState(false)
  const [externalError, setExternalError] = useState('')
  const [searchTotal, setSearchTotal] = useState(0)
  const [nextPage, setNextPage] = useState(2)
  const searchSeq = useRef(0)
  const activeController = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!canUseAI) {
      setAiAvailable(false)
      setAiOn(false)
      return
    }
    aiAPI
      .status()
      .then((status) => setAiAvailable(status.enabled))
      .catch(() => setAiAvailable(false))
  }, [canUseAI])

  useEffect(() => {
    setQ(urlQuery)
  }, [urlQuery])

  useEffect(() => {
    activeController.current?.abort()
    if (aiOn) {
      setLoading(false)
      setLoadingMore(false)
      setExternalLoading(false)
      setExternalError('')
      return
    }

    const query = q.trim()
    const seq = ++searchSeq.current
    if (!query) {
      setLocalCards([])
      setExternalItems([])
      setExternalLoading(false)
      setExternalError('')
      setIntent(null)
      setSearchTotal(0)
      setHasSearched(false)
      setLoading(false)
      setLoadingMore(false)
      setError('')
      return
    }

    const controller = new AbortController()
    activeController.current = controller
    setHasSearched(true)
    setLoading(true)
    setLoadingMore(false)
    setError('')
    setLocalCards([])
    setSearchTotal(0)
    setNextPage(2)
    setExternalItems([])
    setExternalLoading(canViewDiscover)
    setExternalError('')
    setIntent(null)
    const timer = window.setTimeout(() => {
      mediaAPI.searchSeriesPage(query, 1, LOCAL_SEARCH_PAGE_SIZE, controller.signal)
        .then((data) => {
          if (controller.signal.aborted || seq !== searchSeq.current) return
          setLocalCards(data.items ?? [])
          setSearchTotal(data.total ?? (data.items ?? []).length)
          setNextPage(2)
          setIntent(null)
        })
        .catch((err) => {
          if (controller.signal.aborted || seq !== searchSeq.current) return
          setLocalCards([])
          setSearchTotal(0)
          const message = apiErrorMessage(err, '搜索失败')
          setError(message)
          toast.error(message)
        })
        .finally(() => {
          if (!controller.signal.aborted && seq === searchSeq.current) setLoading(false)
        })
      if (canViewDiscover) {
        discoverAPI.search(query, 'all', '', 1, 40, controller.signal)
          .then((data) => {
            if (controller.signal.aborted || seq !== searchSeq.current) return
            setExternalItems(data.items.map((item) => ({ ...item, source: item.source || 'tmdb', subscribe_keyword: item.subscribe_keyword || item.title })))
            setExternalError(data.error)
          })
          .catch((err) => {
            if (!controller.signal.aborted && seq === searchSeq.current) setExternalError(apiErrorMessage(err, '影视资料搜索失败'))
          })
          .finally(() => {
            if (!controller.signal.aborted && seq === searchSeq.current) setExternalLoading(false)
          })
      }
    }, 300)

    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [aiOn, canViewDiscover, q])

  const loadMore = useCallback(async () => {
    const query = q.trim()
    if (!query || loading || loadingMore || localCards.length >= searchTotal) return

    // The initial request may still be loading catalogue results. Loading a
    // local page must not cancel that independent part of the same search.
    const controller = new AbortController()
    activeController.current = controller
    const seq = searchSeq.current
    setLoadingMore(true)
    setError('')
    try {
      const data = await mediaAPI.searchSeriesPage(query, nextPage, LOCAL_SEARCH_PAGE_SIZE, controller.signal)
      if (controller.signal.aborted || seq !== searchSeq.current) return
      setLocalCards((current) => current.concat(data.items ?? []))
      setSearchTotal(data.total ?? searchTotal)
      setNextPage((page) => page + 1)
    } catch (err) {
      if (controller.signal.aborted || seq !== searchSeq.current) return
      const message = apiErrorMessage(err, '加载更多搜索结果失败')
      setError(message)
      toast.error(message)
    } finally {
      if (!controller.signal.aborted && seq === searchSeq.current) setLoadingMore(false)
    }
  }, [loading, loadingMore, localCards.length, nextPage, q, searchTotal])

  const clearQuery = useCallback(() => {
    activeController.current?.abort()
    ++searchSeq.current
    setQ('')
    setLocalCards([])
    setExternalItems([])
    setExternalLoading(false)
    setExternalError('')
    setIntent(null)
    setSearchTotal(0)
    setNextPage(2)
    setHasSearched(false)
    setLoading(false)
    setLoadingMore(false)
    setError('')

    const next = new URLSearchParams(searchParams)
    if (next.has('q')) {
      next.delete('q')
      setSearchParams(next, { replace: true })
    }
  }, [searchParams, setSearchParams])

  const onAISubmit = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedQuery = q.trim()
    if (!trimmedQuery) return
    activeController.current?.abort()
    const seq = ++searchSeq.current
    setLoading(true)
    setLoadingMore(false)
    setError('')
    setHasSearched(true)
    try {
      const data = await aiAPI.smartSearch(trimmedQuery)
      if (seq !== searchSeq.current) return
      const cards = groupSeries(data.items ?? [])
      setLocalCards(cards)
      setSearchTotal(cards.length)
      setExternalItems(data.external_items ?? [])
      setIntent(data.intent)
    } catch (err) {
      if (seq !== searchSeq.current) return
      const message = apiErrorMessage(err, 'AI 搜索失败')
      setError(message)
      toast.error(message)
    } finally {
      if (seq === searchSeq.current) setLoading(false)
    }
  }

  return {
    aiAvailable,
    aiOn,
    clearQuery,
    error,
    externalItems,
    externalLoading,
    externalError,
    hasMore: !aiOn && localCards.length < searchTotal,
    intent,
    itemCount: localCards.length,
    loadMore,
    loading,
    loadingMore,
    localCards,
    onAISubmit,
    q,
    searchTotal,
    setAiOn,
    setQ,
    showEmpty: !loading && !externalLoading && !error && hasSearched && localCards.length === 0 && externalItems.length === 0,
    showIdle: !loading && !error && !hasSearched,
  }
}
