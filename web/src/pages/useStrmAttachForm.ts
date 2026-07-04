import type { FormEvent } from 'react'
import { useState } from 'react'
import toast from 'react-hot-toast'

import { mediaAPI } from '../api/library'
import { strmAPI } from '../api/strm'
import { confirmAction } from '../components/confirmAction'
import type { Media } from '../types'
import { apiErrorMessage, isHTTPURL } from './strmPageUtils'

export function useStrmAttachForm() {
  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState<Media[]>([])
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  const doSearch = async (event?: FormEvent) => {
    event?.preventDefault()
    const trimmedQuery = query.trim()
    if (!trimmedQuery) return
    setSearching(true)
    try {
      const result = await mediaAPI.search(trimmedQuery, 30)
      setResults(result.items ?? [])
    } catch {
      toast.error('搜索失败')
    } finally {
      setSearching(false)
    }
  }

  const onAttach = async (media: Media) => {
    const next = (drafts[media.id] ?? '').trim()
    if (!next) return
    if (!isHTTPURL(next)) {
      toast.error('URL 必须以 http:// 或 https:// 开头')
      return
    }

    try {
      await strmAPI.set(media.id, next)
      toast.success('已设置 STRM URL')
      setResults((rows) =>
        rows.map((item) => (item.id === media.id ? ({ ...item, container: 'strm' } as Media) : item)),
      )
      setDrafts((current) => ({ ...current, [media.id]: '' }))
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '设置失败'))
    }
  }

  const onDetach = async (media: Media) => {
    const ok = await confirmAction({
      title: '清除 STRM URL',
      message: `清除「${media.title}」的 STRM URL?`,
      confirmText: '清除',
    })
    if (!ok) return

    try {
      await strmAPI.clear(media.id)
      toast.success('已清除')
      setResults((rows) =>
        rows.map((item) =>
          item.id === media.id
            ? ({ ...item, container: item.container === 'strm' ? '' : item.container } as Media)
            : item,
        ),
      )
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '清除失败'))
    }
  }

  return {
    doSearch,
    drafts,
    onAttach,
    onDetach,
    query,
    results,
    searching,
    setDrafts,
    setQuery,
  }
}
