import { FormEvent, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'

import { aiAPI, type ExternalMediaResult, type SearchIntent } from '../api/ai'
import type { Media } from '../types'
import { groupSeries } from '../utils/groupSeries'
import type { AIAssistantStatus } from './AIAssistantHeader'

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

export function useAIAssistantPage() {
  const [status, setStatus] = useState<AIAssistantStatus | null>(null)
  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [intent, setIntent] = useState<SearchIntent | null>(null)
  const [items, setItems] = useState<Media[]>([])
  const [externalItems, setExternalItems] = useState<ExternalMediaResult[]>([])
  const localCards = useMemo(() => groupSeries(items), [items])

  const [recs, setRecs] = useState<string[] | null>(null)
  const [recommending, setRecommending] = useState(false)

  useEffect(() => {
    aiAPI
      .status()
      .then(setStatus)
      .catch(() => setStatus({ enabled: false, provider: '', model: '' }))
  }, [])

  const onSearch = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedQuery = query.trim()
    if (!trimmedQuery) return

    setSearching(true)
    setIntent(null)
    setItems([])
    setExternalItems([])
    try {
      const result = await aiAPI.smartSearch(trimmedQuery)
      const externalResults = result.external_items ?? []
      setIntent(result.intent)
      setItems(result.items)
      setExternalItems(externalResults)
      if (result.items.length === 0 && externalResults.length === 0) toast('未找到匹配项')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '搜索失败'))
    } finally {
      setSearching(false)
    }
  }

  const onRecommend = async () => {
    setRecommending(true)
    try {
      const titles = await aiAPI.recommend()
      setRecs(titles)
      if (titles.length === 0) toast('暂无可推荐内容,请先观看一些媒体')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '获取推荐失败'))
    } finally {
      setRecommending(false)
    }
  }

  return {
    externalItems,
    intent,
    items,
    localCards,
    onRecommend,
    onSearch,
    query,
    recommending,
    recs,
    searching,
    setQuery,
    status,
  }
}
