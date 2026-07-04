import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import toast from 'react-hot-toast'

import { mediaAPI, type ManualScrapeCandidate } from '../api/library'
import type { Media } from '../types'
import {
  candidateKey,
  manualSearchProvidersForSelection,
  mergeManualCandidates,
} from './ManualScrapeDialogModel'

interface ManualScrapeDialogStateOptions {
  open: boolean
  media: Media | null
  mediaIds?: string[]
  defaultQuery?: string
  mediaType?: string
  episodeArtwork?: boolean
  onClose: () => void
  onApplied?: () => void
}

interface SearchManualScrapeParams {
  media: Media | null
  mediaType?: string
  query: string
  selectedProviders: string[]
  setSearching: Dispatch<SetStateAction<boolean>>
  setItems: Dispatch<SetStateAction<ManualScrapeCandidate[]>>
}

interface ManualScrapeDialogResetParams {
  open: boolean
  mediaTitle?: string
  defaultQuery?: string
  episodeArtwork?: boolean
  setQuery: Dispatch<SetStateAction<string>>
  setSelectedProviders: Dispatch<SetStateAction<string[]>>
  setIncludeEpisodeArtwork: Dispatch<SetStateAction<boolean>>
  setApplyingKey: Dispatch<SetStateAction<string>>
  setItems: Dispatch<SetStateAction<ManualScrapeCandidate[]>>
}

interface ApplyManualScrapeActionParams {
  media: Media | null
  targetIds: string[]
  includeEpisodeArtwork: boolean
  setApplyingKey: Dispatch<SetStateAction<string>>
  onClose: () => void
  onApplied?: () => void
}

interface ApplyManualScrapeParams extends ApplyManualScrapeActionParams {
  item: ManualScrapeCandidate
}

export function useManualScrapeDialogState({
  open,
  media,
  mediaIds,
  defaultQuery,
  mediaType,
  episodeArtwork,
  onClose,
  onApplied,
}: ManualScrapeDialogStateOptions) {
  const form = useManualScrapeFormState()
  const targetIds = useMemo(() => manualScrapeTargetIds(media, mediaIds), [media, mediaIds])

  useManualScrapeDialogReset({
    open,
    mediaTitle: media?.title,
    defaultQuery,
    episodeArtwork,
    setQuery: form.setQuery,
    setSelectedProviders: form.setSelectedProviders,
    setIncludeEpisodeArtwork: form.setIncludeEpisodeArtwork,
    setApplyingKey: form.setApplyingKey,
    setItems: form.setItems,
  })

  const runSearch = useManualScrapeSearchAction({
    media,
    mediaType,
    query: form.query,
    selectedProviders: form.selectedProviders,
    setSearching: form.setSearching,
    setItems: form.setItems,
  })

  const apply = useManualScrapeApplyAction({
    media,
    targetIds,
    includeEpisodeArtwork: form.includeEpisodeArtwork,
    setApplyingKey: form.setApplyingKey,
    onClose,
    onApplied,
  })

  return { ...form, targetIds, runSearch, apply }
}

function useManualScrapeFormState() {
  const [query, setQuery] = useState('')
  const [selectedProviders, setSelectedProviders] = useState<string[]>([])
  const [includeEpisodeArtwork, setIncludeEpisodeArtwork] = useState(false)
  const [searching, setSearching] = useState(false)
  const [applyingKey, setApplyingKey] = useState('')
  const [items, setItems] = useState<ManualScrapeCandidate[]>([])
  return {
    query,
    selectedProviders,
    includeEpisodeArtwork,
    searching,
    applyingKey,
    items,
    setQuery,
    setSelectedProviders,
    setIncludeEpisodeArtwork,
    setSearching,
    setApplyingKey,
    setItems,
  }
}

function useManualScrapeDialogReset({
  open,
  mediaTitle,
  defaultQuery,
  episodeArtwork,
  setQuery,
  setSelectedProviders,
  setIncludeEpisodeArtwork,
  setApplyingKey,
  setItems,
}: ManualScrapeDialogResetParams): void {
  useEffect(() => {
    if (!open) return
    setQuery(defaultQuery || mediaTitle || '')
    setSelectedProviders([])
    setIncludeEpisodeArtwork(episodeArtwork ?? false)
    setItems([])
    setApplyingKey('')
  }, [
    defaultQuery,
    episodeArtwork,
    mediaTitle,
    open,
    setApplyingKey,
    setIncludeEpisodeArtwork,
    setItems,
    setQuery,
    setSelectedProviders,
  ])
}

function useManualScrapeSearchAction(params: SearchManualScrapeParams): () => Promise<void> {
  const { media, mediaType, query, selectedProviders, setSearching, setItems } = params
  return useCallback(() => searchManualScrapeCandidates({
    media,
    mediaType,
    query,
    selectedProviders,
    setSearching,
    setItems,
  }), [media, mediaType, query, selectedProviders, setSearching, setItems])
}

function useManualScrapeApplyAction({
  media,
  targetIds,
  includeEpisodeArtwork,
  setApplyingKey,
  onClose,
  onApplied,
}: ApplyManualScrapeActionParams): (item: ManualScrapeCandidate) => Promise<void> {
  return useCallback((item) => applyManualScrapeCandidate({
    media,
    targetIds,
    includeEpisodeArtwork,
    item,
    setApplyingKey,
    onClose,
    onApplied,
  }), [includeEpisodeArtwork, media, onApplied, onClose, setApplyingKey, targetIds])
}

function manualScrapeTargetIds(media: Media | null, mediaIds?: string[]): string[] {
  const ids = (mediaIds && mediaIds.length > 0 ? mediaIds : media ? [media.id] : []).filter(Boolean)
  return Array.from(new Set(ids))
}

async function searchManualScrapeCandidates({
  media,
  mediaType,
  query,
  selectedProviders,
  setSearching,
  setItems,
}: SearchManualScrapeParams): Promise<void> {
  if (!media) return
  const text = query.trim()
  if (!text) {
    toast.error('请输入标题或 TMDb/豆瓣/Bangumi/TheTVDB ID')
    return
  }
  setSearching(true)
  setItems([])
  const providerValues = manualSearchProvidersForSelection(selectedProviders, text)
  if (providerValues.length === 0) {
    toast.error('至少选择一个刮削源')
    setSearching(false)
    return
  }
  try {
    const settled = await Promise.allSettled(providerValues.map(async (provider) => {
      const results = await mediaAPI.manualScrapeSearch(media.id, { query: text, provider, media_type: mediaType })
      setItems((current) => mergeManualCandidates(current, results))
      return { provider, results }
    }))
    reportManualScrapeSearchResult(settled, providerValues.length)
  } catch (err: unknown) {
    toast.error(apiErrorMessage(err, '搜索失败'))
  } finally {
    setSearching(false)
  }
}

async function applyManualScrapeCandidate({
  media,
  targetIds,
  includeEpisodeArtwork,
  item,
  setApplyingKey,
  onClose,
  onApplied,
}: ApplyManualScrapeParams): Promise<void> {
  if (!media) return
  setApplyingKey(candidateKey(item))
  try {
    const options = { episode_images: includeEpisodeArtwork }
    if (targetIds.length > 1) {
      const result = await mediaAPI.applyManualScrapeBatch(targetIds, item, options)
      toast.success(`已应用到 ${result.applied} 个媒体`)
    } else {
      await mediaAPI.applyManualScrape(media.id, item, options)
      toast.success('已应用手动匹配')
    }
    onApplied?.()
    onClose()
  } catch (err: unknown) {
    toast.error(apiErrorMessage(err, '应用失败'))
  } finally {
    setApplyingKey('')
  }
}

function reportManualScrapeSearchResult(
  settled: PromiseSettledResult<{ provider: string; results: ManualScrapeCandidate[] }>[],
  providerCount: number,
): void {
  const found = settled.reduce((sum, result) => (
    result.status === 'fulfilled' ? sum + result.value.results.length : sum
  ), 0)
  const failed = settled.filter((result) => result.status === 'rejected').length
  if (found === 0) {
    toast.error(failed === providerCount ? '所有刮削源搜索失败' : '没有找到可用候选')
  } else if (failed > 0) {
    toast.error(`${failed} 个刮削源搜索失败，已显示其余结果`)
  }
}

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback
}
