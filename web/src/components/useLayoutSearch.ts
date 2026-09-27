import { useEffect, useRef, useState, type FormEvent } from 'react'
import type { NavigateFunction } from 'react-router-dom'

import { mediaAPI } from '../api/library'
import type { SeriesCard } from '../utils/groupSeries'

type UseLayoutSearchOptions = {
  pathname: string
  locationSearch: string
  navigate: NavigateFunction
}

export function useLayoutSearch({ pathname, locationSearch, navigate }: UseLayoutSearchOptions) {
  const [focused, setFocused] = useState(false)
  const [query, setQuery] = useState('')
  const [cards, setCards] = useState<SeriesCard[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')
  const searchSeq = useRef(0)

  useEffect(() => {
    setFocused(false)
    if (pathname === '/search') {
      setQuery(new URLSearchParams(locationSearch).get('q') ?? '')
    }
  }, [pathname, locationSearch])

  useEffect(() => {
    const trimmedQuery = query.trim()
    const seq = ++searchSeq.current
    if (!focused || !trimmedQuery) {
      setCards([])
      setTotal(0)
      setError('')
      setLoading(false)
      return
    }

    setLoading(true)
    setError('')
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      mediaAPI
        .searchSeriesPage(trimmedQuery, 1, 8, controller.signal)
        .then((data) => {
          if (seq !== searchSeq.current) return
          setCards(data.items ?? [])
          setTotal(data.total ?? (data.items ?? []).length)
        })
        .catch(() => {
          if (controller.signal.aborted || seq !== searchSeq.current) return
          setCards([])
          setTotal(0)
          setError('搜索失败，请稍后再试')
        })
        .finally(() => {
          if (seq === searchSeq.current) setLoading(false)
        })
    }, 220)

    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [focused, query])

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const trimmedQuery = query.trim()
    if (trimmedQuery) {
      setFocused(false)
      navigate(`/search?q=${encodeURIComponent(trimmedQuery)}`)
    }
  }

  const clear = () => {
    ++searchSeq.current
    setQuery('')
    setCards([])
    setTotal(0)
    setError('')
    setLoading(false)
    setFocused(true)

    if (pathname === '/search') {
      const next = new URLSearchParams(locationSearch)
      if (next.has('q')) {
        next.delete('q')
        const search = next.toString()
        navigate({ pathname, search: search ? `?${search}` : '' }, { replace: true })
      }
    }
  }

  return {
    cards,
    clear,
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
