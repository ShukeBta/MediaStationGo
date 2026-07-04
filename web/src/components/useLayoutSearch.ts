import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import type { NavigateFunction } from 'react-router-dom'

import { mediaAPI } from '../api/library'
import type { Media } from '../types'
import { groupSeries } from '../utils/groupSeries'

type UseLayoutSearchOptions = {
  pathname: string
  locationSearch: string
  navigate: NavigateFunction
}

export function useLayoutSearch({ pathname, locationSearch, navigate }: UseLayoutSearchOptions) {
  const [focused, setFocused] = useState(false)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<Media[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')
  const searchSeq = useRef(0)
  const cards = useMemo(() => groupSeries(items).slice(0, 8), [items])

  useEffect(() => {
    if (pathname === '/search') {
      setQuery(new URLSearchParams(locationSearch).get('q') ?? '')
    }
  }, [pathname, locationSearch])

  useEffect(() => {
    const trimmedQuery = query.trim()
    const seq = ++searchSeq.current
    if (!focused || !trimmedQuery) {
      setItems([])
      setTotal(0)
      setError('')
      setLoading(false)
      return
    }

    setLoading(true)
    setError('')
    const timer = window.setTimeout(() => {
      mediaAPI
        .search(trimmedQuery, 24)
        .then((data) => {
          if (seq !== searchSeq.current) return
          setItems(data.items ?? [])
          setTotal(data.total ?? (data.items ?? []).length)
        })
        .catch(() => {
          if (seq !== searchSeq.current) return
          setItems([])
          setTotal(0)
          setError('搜索失败，请稍后再试')
        })
        .finally(() => {
          if (seq === searchSeq.current) setLoading(false)
        })
    }, 220)

    return () => window.clearTimeout(timer)
  }, [focused, query])

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const trimmedQuery = query.trim()
    if (trimmedQuery) {
      navigate(`/search?q=${encodeURIComponent(trimmedQuery)}`)
      setFocused(false)
    }
  }

  return {
    cards,
    error,
    focused,
    loading,
    query,
    total,
    setFocused,
    setQuery,
    submit,
  }
}
