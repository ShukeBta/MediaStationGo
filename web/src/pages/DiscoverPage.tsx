import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { discoverAPI, type DiscoverItem, type DiscoverSection } from '../api/discover'
import { AdultPerformerModal } from './AdultPerformerModal'
import { ContentRow, DiscoverSkeleton, type DiscoverRefreshStatus } from './DiscoverContentRow'
import { DiscoverDetailModal } from './DiscoverDetailModal'
import { DiscoverEmptySelection, DiscoverHeader, DiscoverResults } from './DiscoverPageSections'
import { DiscoverSectionPickerModal } from './DiscoverSectionPickerModal'
import {
  defaultSections,
  fd2PPVSortOptions,
  orderSelectedSections,
  readCachedDiscoverRows,
  writeCachedDiscoverRow,
} from './discoverPageModel'

type DiscoverModalEntry = {
	id: number
	item: DiscoverItem
}

export function DiscoverPage() {
  const [sections, setSections] = useState<DiscoverSection[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [rows, setRows] = useState<Record<string, DiscoverItem[]>>({})
  const [rowPages, setRowPages] = useState<Record<string, number>>({})
  const [rowCanNext, setRowCanNext] = useState<Record<string, boolean>>({})
  const [rowSorts, setRowSorts] = useState<Record<string, string>>({ adult_fd2ppv: 'release' })
  const [rowSortSaving, setRowSortSaving] = useState<Record<string, boolean>>({})
  const [rowLoading, setRowLoading] = useState<Record<string, boolean>>({})
  const [rowRefreshStatus, setRowRefreshStatus] = useState<Record<string, DiscoverRefreshStatus>>({})
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({})
  const [sectionsReady, setSectionsReady] = useState(false)
  const [selectionSaving, setSelectionSaving] = useState(false)
  const [selectionError, setSelectionError] = useState('')
	const [sectionPickerOpen, setSectionPickerOpen] = useState(false)
	const [sectionPickerDraft, setSectionPickerDraft] = useState<string[]>([])
  const [modalStack, setModalStack] = useState<DiscoverModalEntry[]>([])
	const [searchQuery, setSearchQuery] = useState('')
	const [searchItems, setSearchItems] = useState<DiscoverItem[]>([])
	const [searchLoading, setSearchLoading] = useState(false)
	const [searchErrors, setSearchErrors] = useState<Record<string, string>>({})
	const [searchDone, setSearchDone] = useState(false)
	const searchSequence = useRef(0)
	const modalSequence = useRef(0)
  const rowPagesRef = useRef<Record<string, number>>({})
  const rowSortsRef = useRef<Record<string, string>>({ adult_fd2ppv: 'release' })
	const rowRequestSequences = useRef<Record<string, number>>({})
	const rowRefreshClearTimers = useRef<Record<string, number>>({})

	const loadDiscoverSection = useCallback(async (
		key: string,
		page: number,
		refresh = false,
		sortOverride?: string,
	): Promise<boolean | undefined> => {
		const sequence = (rowRequestSequences.current[key] ?? 0) + 1
		rowRequestSequences.current[key] = sequence
		setRowLoading((current) => ({ ...current, [key]: true }))
		setRowErrors((current) => updateDiscoverRowError(current, key))
		try {
			const adultFD2PPVSort = key === 'adult_fd2ppv'
				? sortOverride || rowSortsRef.current[key] || 'release'
				: undefined
			const feed = await discoverAPI.feed([key], page, { refresh, adultFD2PPVSort })
			if (rowRequestSequences.current[key] !== sequence) return undefined
			const meta = feed.meta[key]
			const issue = meta?.error || meta?.warning
			const nextItems = feed.items[key] ?? []
			const nextCanNext = Boolean(meta?.has_next)
			setRows((current) => {
				if (meta?.error && nextItems.length === 0 && (current[key]?.length ?? 0) > 0) {
					return current
				}
				return { ...current, [key]: nextItems }
			})
			setRowCanNext((current) => {
				if (meta?.error && nextItems.length === 0 && key in current) {
					return current
				}
				return { ...current, [key]: nextCanNext }
			})
			if (!issue && (key !== 'adult_fd2ppv' || adultFD2PPVSort === 'release')) {
				writeCachedDiscoverRow(key, page, nextItems, nextCanNext)
			}
			setRowErrors((current) => updateDiscoverRowError(current, key, issue))
			return !issue
		} catch (error) {
			if (rowRequestSequences.current[key] !== sequence) return undefined
			const message = discoverRequestErrorMessage(error)
			setRows((current) => ((current[key]?.length ?? 0) > 0 ? current : { ...current, [key]: [] }))
			setRowCanNext((current) => (key in current ? current : { ...current, [key]: false }))
			setRowErrors((current) => ({ ...current, [key]: message }))
			return false
		} finally {
			if (rowRequestSequences.current[key] === sequence) {
				setRowLoading((current) => ({ ...current, [key]: false }))
			}
		}
	}, [])

  useEffect(() => () => {
    for (const timer of Object.values(rowRefreshClearTimers.current)) {
      window.clearTimeout(timer)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    setSectionsReady(false)
    Promise.all([discoverAPI.sections(), discoverAPI.preference()])
      .then(async ([items, preference]) => {
        if (cancelled) return
        const available = new Set(items.map((item) => item.key))
        const fallback = defaultSections.filter((key) => available.has(key))
        const saved = preference.selected_sections.filter((key) => available.has(key))
        const nextSelected = orderSelectedSections(preference.configured ? saved : fallback, items)
        let savedPreference = preference
        if (!preference.configured) {
          savedPreference = await discoverAPI.savePreference(nextSelected, preference.adult_fd2ppv_sort)
        }
        if (cancelled) return
        const savedFD2PPVSort = savedPreference.adult_fd2ppv_sort
        const nextSorts = { adult_fd2ppv: savedFD2PPVSort }
        rowSortsRef.current = nextSorts
        setRowSorts(nextSorts)
        setSections(items)
        const cached = readCachedDiscoverRows(nextSelected.filter(
          (key) => key !== 'adult_fd2ppv' || savedFD2PPVSort === 'release',
        ))
        setSelected(nextSelected)
        const nextPages = Object.fromEntries(nextSelected.map((key) => [key, 1]))
        rowPagesRef.current = nextPages
        setRowPages(nextPages)
        setRows(cached.rows)
        setRowCanNext(cached.rowCanNext)
        setSectionsReady(true)
      })
      .catch((error) => {
        if (cancelled) return
        setSections([])
        setSelected([])
        setSelectionError(discoverPreferenceErrorMessage(error))
        setSectionsReady(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!sectionsReady) return
    const available = new Set(sections.map((section) => section.key))
    const activeSelected = orderSelectedSections(
      selected.filter((key) => available.has(key)),
      sections,
    )
    if (
      activeSelected.length !== selected.length ||
      activeSelected.some((key, index) => key !== selected[index])
    ) {
      setSelected(activeSelected)
      return
    }
    if (selected.length === 0) {
      for (const key of Object.keys(rowRequestSequences.current)) {
        rowRequestSequences.current[key] += 1
      }
      setRows({})
      setRowLoading({})
      setRowCanNext({})
      setRowErrors({})
      return
    }
		const requestSequences = rowRequestSequences.current
    setRows((current) => Object.fromEntries(selected.map((key) => [key, current[key] ?? []])))
    setRowCanNext((current) => Object.fromEntries(
      selected.filter((key) => key in current).map((key) => [key, current[key]]),
    ))
    setRowErrors((current) => Object.fromEntries(
      selected.filter((key) => key in current).map((key) => [key, current[key]]),
    ))
    setRowRefreshStatus((current) => Object.fromEntries(
      selected.filter((key) => key in current).map((key) => [key, current[key]]),
    ))
    for (const key of selected) {
      void loadDiscoverSection(key, rowPagesRef.current[key] ?? 1)
    }
    return () => {
      for (const key of selected) {
        requestSequences[key] = (requestSequences[key] ?? 0) + 1
      }
    }
  }, [loadDiscoverSection, sections, sectionsReady, selected])

  const sectionMap = useMemo(
    () => new Map(sections.map((section) => [section.key, section])),
    [sections],
  )
  const loading = selected.some((key) => Boolean(rowLoading[key]))
  const hasContent = selected.some((key) => (rows[key] ?? []).length > 0)
  const sectionLabel = (key: string) => sectionMap.get(key)?.label ?? key
	const searchGroups = useMemo(() => groupDiscoverSearchItems(searchItems), [searchItems])
	const searchActive = searchLoading || searchDone
	const showSearchArea = sectionsReady && searchActive
	const adultSearchAvailable = sections.some((section) => section.group === 'adult')

  const openSectionPicker = () => {
		setSectionPickerDraft(selected)
		setSelectionError('')
		setSectionPickerOpen(true)
	}

	const toggleSectionPickerDraft = (key: string) => {
		setSectionPickerDraft((current) => orderSelectedSections(
			current.includes(key) ? current.filter((item) => item !== key) : [...current, key],
			sections,
		))
	}

	const reorderSectionPickerDraft = (keys: string[]) => {
		setSectionPickerDraft(orderSelectedSections(keys, sections))
	}

	const saveSectionSelection = async () => {
    if (selectionSaving) return
		const next = orderSelectedSections(sectionPickerDraft, sections)
		if (next.length === selected.length && next.every((key, index) => key === selected[index])) {
			setSectionPickerOpen(false)
			return
		}
    setSelectionSaving(true)
    setSelectionError('')
    try {
      const saved = await discoverAPI.savePreference(next, rowSortsRef.current.adult_fd2ppv)
		const savedSelection = orderSelectedSections(saved.selected_sections, sections)
      setSelected(savedSelection)
		setSectionPickerDraft(savedSelection)
		setRowPages((current) => {
			const nextPages = Object.fromEntries(savedSelection.map((key) => [key, current[key] ?? 1]))
			rowPagesRef.current = nextPages
			return nextPages
		})
		setSectionPickerOpen(false)
    } catch (error) {
      setSelectionError(discoverPreferenceErrorMessage(error))
    } finally {
      setSelectionSaving(false)
    }
  }

  const changeDiscoverPage = (key: string, delta: number) => {
    const currentPage = rowPagesRef.current[key] ?? 1
    const nextPage = Math.max(1, currentPage + delta)
    if (nextPage === currentPage) return
    const nextPages = { ...rowPagesRef.current, [key]: nextPage }
    rowPagesRef.current = nextPages
    setRowPages(nextPages)
    void loadDiscoverSection(key, nextPage)
  }

  const changeDiscoverSort = async (key: string, sort: string) => {
    if (key !== 'adult_fd2ppv' || !fd2PPVSortOptions.some((option) => option.value === sort)) return
    if ((rowSortsRef.current[key] || 'release') === sort) return
    setRowErrors((current) => updateDiscoverRowError(current, key))
    setRowSortSaving((current) => ({ ...current, [key]: true }))
    try {
      const saved = await discoverAPI.savePreference(selected, sort)
      const savedSort = saved.adult_fd2ppv_sort
      const nextSorts = { ...rowSortsRef.current, [key]: savedSort }
      rowSortsRef.current = nextSorts
      setRowSorts(nextSorts)
      const nextPages = { ...rowPagesRef.current, [key]: 1 }
      rowPagesRef.current = nextPages
      setRowPages(nextPages)
      setRows((current) => ({ ...current, [key]: [] }))
      setRowCanNext((current) => ({ ...current, [key]: false }))
      await loadDiscoverSection(key, 1, false, savedSort)
    } catch (error) {
      setRowErrors((current) => ({ ...current, [key]: discoverSortPreferenceErrorMessage(error) }))
    } finally {
      setRowSortSaving((current) => ({ ...current, [key]: false }))
    }
  }

  const refreshDiscoverSection = (key: string) => {
    const existingTimer = rowRefreshClearTimers.current[key]
    if (existingTimer) {
      window.clearTimeout(existingTimer)
      delete rowRefreshClearTimers.current[key]
    }
    setRowRefreshStatus((current) => ({ ...current, [key]: 'loading' }))
    void loadDiscoverSection(key, rowPagesRef.current[key] ?? 1, true).then((success) => {
      if (success === undefined) {
        setRowRefreshStatus((current) => {
          if (current[key] !== 'loading') return current
          const next = { ...current }
          delete next[key]
          return next
        })
        return
      }
      const status: DiscoverRefreshStatus = success ? 'success' : 'error'
      setRowRefreshStatus((current) => ({ ...current, [key]: status }))
      rowRefreshClearTimers.current[key] = window.setTimeout(() => {
        delete rowRefreshClearTimers.current[key]
        setRowRefreshStatus((current) => {
          if (current[key] !== status) return current
          const next = { ...current }
          delete next[key]
          return next
        })
      }, 2500)
    })
  }

	const searchDiscoverCatalog = async () => {
		const query = searchQuery.trim()
		const sequence = searchSequence.current + 1
		searchSequence.current = sequence
		setSearchDone(true)
		setSearchItems([])
		setSearchErrors({})
		if ([...query].length < 1) {
			setSearchLoading(false)
			setSearchErrors({ request: '请输入搜索词' })
			return
		}
		setSearchLoading(true)
		try {
			const result = await discoverAPI.search(query)
			if (searchSequence.current !== sequence) return
			setSearchItems(result.items)
			setSearchErrors(result.errors)
		} catch (error) {
			if (searchSequence.current !== sequence) return
			const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
			setSearchErrors({ request: message || '聚合搜索失败' })
		} finally {
			if (searchSequence.current === sequence) setSearchLoading(false)
		}
	}

	const clearDiscoverSearch = () => {
		searchSequence.current += 1
		setSearchQuery('')
		setSearchItems([])
		setSearchErrors({})
		setSearchLoading(false)
		setSearchDone(false)
	}

	const openRootModal = (item: DiscoverItem) => {
		modalSequence.current += 1
		setModalStack([{ id: modalSequence.current, item }])
	}

	const pushModal = (item: DiscoverItem) => {
		modalSequence.current += 1
		const entry = { id: modalSequence.current, item }
		setModalStack((current) => [...current, entry])
	}

	const closeTopModal = () => {
		setModalStack((current) => current.slice(0, -1))
	}

	const handleAdultFollowChanged = (performer: DiscoverItem, followed: boolean) => {
		const source = performer.source
		const sourceID = performer.provider_id
		setSearchItems((current) => current.map((item) => (
			item.source === source && item.provider_id === sourceID ? { ...item, followed } : item
		)))
		setModalStack((current) => current.map((entry) => (
			entry.item.source === source && entry.item.provider_id === sourceID
				? { ...entry, item: { ...entry.item, followed } }
				: entry
		)))
		for (const key of ['adult_followed_performers', 'adult_followed']) {
			if (selected.includes(key)) refreshDiscoverSection(key)
		}
	}

  return (
    <div className="mx-auto w-full max-w-[1680px] space-y-10 px-4 py-6 md:px-6 md:py-8">
      <DiscoverHeader
        selectedCount={selected.length}
        sectionsReady={sectionsReady}
		selectionSaving={selectionSaving}
		searchQuery={searchQuery}
        searchLoading={searchLoading}
        searchActive={searchActive}
		adultSearchAvailable={adultSearchAvailable}
		onOpenSectionPicker={openSectionPicker}
		onSearchQueryChange={setSearchQuery}
		onSearch={() => void searchDiscoverCatalog()}
        onClearSearch={clearDiscoverSearch}
      />

		{sectionPickerOpen && (
			<DiscoverSectionPickerModal
				sections={sections}
				selected={sectionPickerDraft}
				saving={selectionSaving}
				error={selectionError}
				onToggle={toggleSectionPickerDraft}
				onReorder={reorderSectionPickerDraft}
				onClose={() => setSectionPickerOpen(false)}
				onSave={() => void saveSectionSelection()}
			/>
		)}

      {!sectionsReady && <div className="xl:ml-[252px]"><DiscoverSkeleton /></div>}

      {selectionError && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {selectionError}
        </div>
      )}

		{showSearchArea && <div className="space-y-8">
			{sectionsReady && Object.keys(searchErrors).length > 0 && (
				<div className="space-y-1 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
					<p className="font-semibold">以下搜索源未返回结果：</p>
					{Object.entries(searchErrors).map(([source, message]) => <p key={source}>{message}</p>)}
				</div>
			)}

			{sectionsReady && searchLoading && (
				<div className="rounded-lg border border-primary-100 bg-primary-50 px-4 py-3 text-sm text-brand-500">
					正在聚合搜索 TMDb、豆瓣、Bangumi{adultSearchAvailable ? '、JavDB 与 FC2' : ''}…
				</div>
			)}

			{sectionsReady && searchGroups.map((group, index) => (
				<ContentRow
					key={group.key}
					title={`搜索结果 · ${group.label}`}
					items={group.items}
					priority={index === 0}
					onSelect={openRootModal}
				/>
			))}

			{sectionsReady && searchDone && !searchLoading && Object.keys(searchErrors).length === 0 && searchItems.length === 0 && (
				<div className="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-500">
					{adultSearchAvailable
						? '没有找到匹配的电影、剧集、动漫、女优或成人作品'
						: '没有找到匹配的电影、剧集或动漫'}
				</div>
			)}
		</div>}

      {sectionsReady && !loading && selected.length === 0 && !searchActive && (
		<div className="xl:ml-[252px]"><DiscoverEmptySelection /></div>
	  )}

      {sectionsReady && selected.length > 0 && !searchActive && (
        <DiscoverResults
          selected={selected}
          rows={rows}
          rowLoading={rowLoading}
          rowRefreshStatus={rowRefreshStatus}
          rowErrors={rowErrors}
          rowPages={rowPages}
          rowCanNext={rowCanNext}
          rowSorts={rowSorts}
          rowSortSaving={rowSortSaving}
          loading={loading}
          hasContent={hasContent}
          sectionLabel={sectionLabel}
          onPageChange={changeDiscoverPage}
          onRefresh={refreshDiscoverSection}
          onSortChange={(key, sort) => void changeDiscoverSort(key, sort)}
          onSelect={openRootModal}
        />
      )}

		{modalStack.map((entry, index) => {
			const active = index === modalStack.length - 1
			return (
				<div key={entry.id} className={active ? undefined : 'hidden'} aria-hidden={!active}>
					{entry.item.media_type === 'person' ? (
						<AdultPerformerModal
							item={entry.item}
							onClose={closeTopModal}
							onSelectWork={pushModal}
							onFollowChanged={(followed) => handleAdultFollowChanged(entry.item, followed)}
						/>
					) : (
						<DiscoverDetailModal
							item={entry.item}
							onClose={closeTopModal}
							onSelectPerformer={pushModal}
						/>
					)}
				</div>
			)
		})}
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

function groupDiscoverSearchItems(items: DiscoverItem[]): Array<{
	key: string
	label: string
	items: DiscoverItem[]
}> {
	const definitions = [
		{ key: 'movie', label: '电影' },
		{ key: 'tv', label: '剧集' },
		{ key: 'anime', label: '动漫' },
		{ key: 'person', label: '女优' },
		{ key: 'adult', label: '成人作品' },
	]
	const grouped = new Map<string, DiscoverItem[]>()
	for (const item of items) {
		const mediaType = item.media_type?.trim().toLowerCase() || 'other'
		grouped.set(mediaType, [...(grouped.get(mediaType) ?? []), item])
	}
	const groups = definitions
		.map((definition) => ({ ...definition, items: grouped.get(definition.key) ?? [] }))
		.filter((group) => group.items.length > 0)
	const known = new Set(definitions.map((definition) => definition.key))
	const otherItems = items.filter((item) => !known.has(item.media_type?.trim().toLowerCase() || 'other'))
	if (otherItems.length > 0) groups.push({ key: 'other', label: '其他', items: otherItems })
	return groups
}

function discoverPreferenceErrorMessage(error: unknown): string {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  return message ? `发现模块设置保存失败：${message}` : '发现模块设置无法从数据库读取或保存，请稍后重试'
}

function discoverSortPreferenceErrorMessage(error: unknown): string {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  return message ? `FC2 排序设置保存失败：${message}` : 'FC2 排序设置无法保存到数据库，请稍后重试'
}
