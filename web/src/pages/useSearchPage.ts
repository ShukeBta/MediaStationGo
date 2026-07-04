import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import toast from 'react-hot-toast'

import { aiAPI, type ExternalMediaResult, type SearchIntent } from '../api/ai'
import { mediaAPI } from '../api/library'
import type { Media } from '../types'
import { groupSeries } from '../utils/groupSeries'

const LOCAL_SEARCH_PAGE_SIZE = 2000

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

export function useSearchPage() {
  const [searchParams] = useSearchParams()
  const urlQuery = searchParams.get('q') ?? ''
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Media[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [aiOn, setAiOn] = useState(false)
  const [aiAvailable, setAiAvailable] = useState(false)
  const [intent, setIntent] = useState<SearchIntent | null>(null)
  const [hasSearched, setHasSearched] = useState(false)
  const [externalItems, setExternalItems] = useState<ExternalMediaResult[]>([])
  const [searchTotal, setSearchTotal] = useState(0)
  const searchSeq = useRef(0)
  const localCards = useMemo(() => groupSeries(items), [items])

  useEffect(() => {
    aiAPI
      .status()
      .then((status) => setAiAvailable(status.enabled))
      .catch(() => setAiAvailable(false))
  }, [])

  useEffect(() => {
    setQ(urlQuery)
  }, [urlQuery])

  const doQuickSearch = useCallback((query: string) => {
    const seq = ++searchSeq.current
    if (!query.trim()) {
      setItems([])
      setSearchTotal(0)
      setHasSearched(false)
      setLoading(false)
      return
    }

    setHasSearched(true)
    setError('')
    const loadAll = async () => {
      let page = 1
      let collected: Media[] = []
      for (;;) {
        const data = await mediaAPI.searchPage(query, page, LOCAL_SEARCH_PAGE_SIZE)
        if (seq !== searchSeq.current) return
        const pageItems = data.items ?? []
        collected = collected.concat(pageItems)
        const total = data.total ?? collected.length
        setSearchTotal(total)
        if (page === 1) setItems(collected)
        if (collected.length >= total || pageItems.length < LOCAL_SEARCH_PAGE_SIZE) break
        page += 1
      }
      if (seq !== searchSeq.current) return
      setItems(collected)
      setExternalItems([])
      setIntent(null)
    }
    loadAll()
      .catch((err) => {
        if (seq !== searchSeq.current) return
        const msg = apiErrorMessage(err, '搜索失败')
        setError(msg)
        toast.error(msg)
      })
      .finally(() => {
        if (seq === searchSeq.current) setLoading(false)
      })
  }, [])

  useEffect(() => {
    if (aiOn) return
    setLoading(true)
    const timer = window.setTimeout(() => doQuickSearch(q), 300)
    return () => window.clearTimeout(timer)
  }, [q, aiOn, doQuickSearch])

  const onAISubmit = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedQuery = q.trim()
    if (!trimmedQuery) return
    ++searchSeq.current
    setLoading(true)
    setError('')
    setHasSearched(true)
    try {
      const data = await aiAPI.smartSearch(q)
      setItems(data.items ?? [])
      setSearchTotal((data.items ?? []).length)
      setExternalItems(data.external_items ?? [])
      setIntent(data.intent)
    } catch (err) {
      const msg = apiErrorMessage(err, 'AI 搜索失败')
      setError(msg)
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }

  return {
    aiAvailable,
    aiOn,
    error,
    externalItems,
    intent,
    itemCount: items.length,
    loading,
    localCards,
    onAISubmit,
    q,
    searchTotal,
    setAiOn,
    setQ,
    showEmpty: !loading && !error && hasSearched && localCards.length === 0,
    showIdle: !loading && !error && !hasSearched,
  }
}
