import { useEffect, useState } from 'react'

export interface LoginShowcaseItem {
  title: string
  overview: string
  artwork_url: string
  year: number
}

export function useLoginShowcase() {
  const [items, setItems] = useState<LoginShowcaseItem[]>([])
  const [index, setIndex] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    void fetch('/api/auth/showcase', { signal: controller.signal })
      .then(async response => {
        if (!response.ok) return
        const data = await response.json() as { items?: LoginShowcaseItem[] }
        if (!controller.signal.aborted && Array.isArray(data.items)) setItems(data.items)
      }).catch(() => { /* The login form remains available when artwork is unavailable. */ })
    return () => controller.abort()
  }, [])
  const current = items.length ? items[index % items.length] : null
  return { current, count: items.length, next: () => setIndex(value => value + 1) }
}
