import { useEffect, useMemo, useState } from 'react'

import { discoverAPI, type DiscoverItem, type DiscoverSection } from '../api/discover'
import { DiscoverSkeleton } from './DiscoverContentRow'
import { DiscoverDetailModal } from './DiscoverDetailModal'
import { DiscoverEmptySelection, DiscoverHeader, DiscoverResults } from './DiscoverPageSections'
import {
  defaultSections,
  discoverStorageKey,
  readCachedDiscoverRows,
  readSavedSections,
  serializeSavedSections,
  writeCachedDiscoverRow,
} from './discoverPageModel'

export function DiscoverPage() {
  const [sections, setSections] = useState<DiscoverSection[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [rows, setRows] = useState<Record<string, DiscoverItem[]>>({})
  const [rowPages, setRowPages] = useState<Record<string, number>>({})
  const [rowCanNext, setRowCanNext] = useState<Record<string, boolean>>({})
  const [rowLoading, setRowLoading] = useState<Record<string, boolean>>({})
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({})
  const [sectionsReady, setSectionsReady] = useState(false)
  const [loading, setLoading] = useState(false)
  const [activeItem, setActiveItem] = useState<DiscoverItem | null>(null)
  const [reloadSeq, setReloadSeq] = useState(0)
  const [imageVersion, setImageVersion] = useState(() => String(Date.now()))
  const [refreshImageVersion, setRefreshImageVersion] = useState<string>()

  useEffect(() => {
    let cancelled = false
    setSectionsReady(false)
    discoverAPI
      .sections()
      .then((items) => {
        if (cancelled) return
        setSections(items)
        const saved = readSavedSections(items)
        const available = new Set(items.map((item) => item.key))
        const fallback = defaultSections.filter((key) => available.has(key))
        const nextSelected = saved.length > 0 ? saved : fallback
        const cached = readCachedDiscoverRows(nextSelected)
        setSelected(nextSelected)
        setRowPages(Object.fromEntries(nextSelected.map((key) => [key, 1])))
        setRows(cached.rows)
        setRowCanNext(cached.rowCanNext)
        setSectionsReady(true)
      })
      .catch(() => {
        if (cancelled) return
        setSections([])
        setSelected([])
        setSectionsReady(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!sectionsReady) return
    const available = new Set(sections.map((section) => section.key))
    const activeSelected = selected.filter((key) => available.has(key))
    if (activeSelected.length !== selected.length) {
      setSelected(activeSelected)
      return
    }
    if (selected.length === 0) {
      setRows({})
      setRowLoading({})
      setRowCanNext({})
      setRowErrors({})
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    setRowErrors({})
    setRowLoading(Object.fromEntries(selected.map((key) => [key, true])))
    setRows((current) => {
      const next: Record<string, DiscoverItem[]> = {}
      for (const key of selected) {
        next[key] = current[key] ?? []
      }
      return next
    })
    window.localStorage.setItem(discoverStorageKey, serializeSavedSections(selected))

    let pending = selected.length
    const markDone = () => {
      pending -= 1
      if (!cancelled && pending <= 0) setLoading(false)
    }
    for (const key of selected) {
      const page = rowPages[key] ?? 1
      discoverAPI
        .feed([key], page)
        .then((feed) => {
          if (cancelled) return
          const error = feed.meta[key]?.error
          const nextItems = feed.items[key] ?? []
          const nextCanNext = Boolean(feed.meta[key]?.has_next)
          setRows((current) => {
            if (error && nextItems.length === 0 && (current[key]?.length ?? 0) > 0) {
              return current
            }
            return { ...current, [key]: nextItems }
          })
          setRowCanNext((current) => {
            if (error && nextItems.length === 0 && key in current) {
              return current
            }
            return { ...current, [key]: nextCanNext }
          })
          if (!error) {
            writeCachedDiscoverRow(key, page, nextItems, nextCanNext)
          }
          setRowErrors((current) => updateDiscoverRowError(current, key, error))
        })
        .catch((err) => {
          if (cancelled) return
          const message = discoverRequestErrorMessage(err)
          setRows((current) => ((current[key]?.length ?? 0) > 0 ? current : { ...current, [key]: [] }))
          setRowCanNext((current) => (key in current ? current : { ...current, [key]: false }))
          setRowErrors((current) => ({ ...current, [key]: message }))
        })
        .finally(() => {
          if (!cancelled) {
            setRowLoading((current) => ({ ...current, [key]: false }))
          }
          markDone()
        })
    }
    return () => {
      cancelled = true
    }
  }, [sections, sectionsReady, selected, rowPages, reloadSeq])

  const sectionMap = useMemo(
    () => new Map(sections.map((section) => [section.key, section])),
    [sections],
  )
  const hasContent = selected.some((key) => (rows[key] ?? []).length > 0)
  const sectionLabel = (key: string) => sectionMap.get(key)?.label ?? key

  const toggleSection = (key: string) => {
    setSelected((current) => {
      if (current.includes(key)) {
        return current.filter((item) => item !== key)
      }
      return [...current, key]
    })
    setRowPages((current) => ({ ...current, [key]: current[key] ?? 1 }))
  }

  const changeDiscoverPage = (key: string, delta: number) => {
    setRowPages((current) => {
      const nextPage = Math.max(1, (current[key] ?? 1) + delta)
      if (nextPage === (current[key] ?? 1)) return current
      return { ...current, [key]: nextPage }
    })
  }

  const refreshDiscover = () => {
    const nextImageVersion = String(Date.now())
    setImageVersion(nextImageVersion)
    setRefreshImageVersion(nextImageVersion)
    setReloadSeq((current) => current + 1)
  }

  return (
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-6">
      <DiscoverHeader
        sections={sections}
        selected={selected}
        sectionsReady={sectionsReady}
        loading={loading}
        onRefresh={refreshDiscover}
        onToggleSection={toggleSection}
      />

      {!sectionsReady && <DiscoverSkeleton />}

      {sectionsReady && !loading && selected.length === 0 && (
        <DiscoverEmptySelection />
      )}

      {sectionsReady && selected.length > 0 && (
        <DiscoverResults
          selected={selected}
          rows={rows}
          rowLoading={rowLoading}
          rowErrors={rowErrors}
          rowPages={rowPages}
          rowCanNext={rowCanNext}
          loading={loading}
          hasContent={hasContent}
          imageVersion={imageVersion}
          refreshImageVersion={refreshImageVersion}
          sectionLabel={sectionLabel}
          onPageChange={changeDiscoverPage}
          onSelect={setActiveItem}
        />
      )}

      {activeItem && (
        <DiscoverDetailModal
          item={activeItem}
          onClose={() => setActiveItem(null)}
        />
      )}
    </div>
  )
}

function updateDiscoverRowError(
  current: Record<string, string>,
  key: string,
  error?: string,
): Record<string, string> {
  if (error) return { ...current, [key]: error }
  if (!(key in current)) return current
  const next = { ...current }
  delete next[key]
  return next
}

function discoverRequestErrorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err)
  const lower = raw.toLowerCase()
  if (lower.includes('timeout') || lower.includes('deadline')) {
    return '推荐源请求超时，已跳过本次加载'
  }
  if (lower.includes('network')) {
    return '推荐源网络不可用，已跳过本次加载'
  }
  return '推荐源暂时不可用，已跳过本次加载'
}
