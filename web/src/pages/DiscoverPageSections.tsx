import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, ArrowLeft, ArrowUp, Layers3, List, LoaderCircle, Search, Sparkles } from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import {
  ContentRow,
  DiscoverCardPlaceholder,
  DiscoverRefreshControl,
  discoverModulePageSize,
  type DiscoverRefreshStatus,
} from './DiscoverContentRow'
import { fd2PPVSortOptions } from './discoverPageModel'

type SectionLabel = (key: string) => string

export function DiscoverHeader({
  selectedCount,
  sectionsReady,
	selectionSaving,
	searchQuery,
  searchLoading,
  searchActive,
	adultSearchAvailable,
	onOpenSectionPicker,
	onSearchQueryChange,
	onSearch,
  onClearSearch,
}: {
  selectedCount: number
  sectionsReady: boolean
	selectionSaving: boolean
  searchQuery: string
  searchLoading: boolean
  searchActive: boolean
	adultSearchAvailable: boolean
	onOpenSectionPicker: () => void
	onSearchQueryChange: (value: string) => void
	onSearch: () => void
  onClearSearch: () => void
}) {
  return (
	<header className="grid items-start gap-6 border-b border-gray-200/80 pb-6 xl:grid-cols-[220px_minmax(0,1fr)] xl:gap-8">
		<div className="min-w-0">
			<div className="flex items-center gap-3">
				<Sparkles className="h-6 w-6 shrink-0 text-brand-500" />
				<h1 className="font-display text-4xl font-bold tracking-tight text-ink-600">发现</h1>
				<button
					type="button"
					onClick={onOpenSectionPicker}
					disabled={!sectionsReady || selectionSaving}
					aria-label="选择发现模块"
					className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-2.5 text-xs font-semibold text-ink-600 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-50"
				>
					<Layers3 size={14} />
					模块
					<span className="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] text-sand-500">{selectedCount}</span>
				</button>
			</div>
			<p className="mt-2 text-sm leading-6 text-ink-50">多源内容推荐，可按需组合显示</p>
		</div>
		<form
			onSubmit={(event) => {
				event.preventDefault()
				onSearch()
			}}
			className="flex w-full flex-col gap-2 sm:flex-row sm:items-center"
		>
			<input
				type="search"
				value={searchQuery}
				onChange={(event) => onSearchQueryChange(event.target.value)}
				placeholder={adultSearchAvailable ? '搜索电影、剧集、动漫、女优或番号' : '搜索电影、剧集或动漫'}
				aria-label="聚合搜索"
				className="h-11 min-w-0 flex-1 rounded-xl border border-gray-200 bg-white px-4 text-base text-ink-600 outline-none transition focus:border-primary-400 focus:ring-2 focus:ring-primary-100 sm:text-sm"
			/>
			<button
				type="submit"
				disabled={!sectionsReady || searchLoading || searchQuery.trim().length < 1}
				className="inline-flex h-11 shrink-0 items-center justify-center gap-1.5 rounded-xl border border-primary-300 bg-primary-500/10 px-4 text-xs font-semibold text-brand-500 transition hover:bg-primary-500/15 disabled:cursor-not-allowed disabled:opacity-50"
			>
				{searchLoading ? <LoaderCircle size={14} className="animate-spin" /> : <Search size={14} />}
				聚合搜索
			</button>
			{searchActive && (
				<button
					type="button"
					onClick={onClearSearch}
					className="inline-flex h-11 shrink-0 items-center justify-center gap-1.5 rounded-xl border border-gray-200 bg-white px-4 text-xs font-semibold text-ink-600 transition hover:border-primary-300 hover:text-brand-500"
				>
					<ArrowLeft size={14} />
					返回发现
				</button>
			)}
		</form>
	</header>
  )
}

export function DiscoverEmptySelection() {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-10 text-center text-sand-500">
      至少选择一个推荐源，小宇宙才会开始转动。
    </div>
  )
}

export function DiscoverResults({
  selected,
  rows,
  rowLoading,
  rowErrors,
  rowRefreshStatus,
  rowPages,
  rowCanNext,
  rowSorts,
  rowSortSaving,
  loading,
  hasContent,
  sectionLabel,
  onPageChange,
  onRefresh,
  onSortChange,
  onSelect,
}: {
  selected: string[]
  rows: Record<string, DiscoverItem[]>
  rowLoading: Record<string, boolean>
  rowErrors: Record<string, string>
  rowRefreshStatus: Record<string, DiscoverRefreshStatus>
  rowPages: Record<string, number>
  rowCanNext: Record<string, boolean>
  rowSorts: Record<string, string>
  rowSortSaving: Record<string, boolean>
  loading: boolean
  hasContent: boolean
  sectionLabel: SectionLabel
  onPageChange: (key: string, delta: number) => void
  onRefresh: (key: string) => void
  onSortChange: (key: string, sort: string) => void
  onSelect: (item: DiscoverItem) => void
}) {
  const sectionTopOffset = 32
  const hasRowErrors = Object.keys(rowErrors).length > 0
  const rowRefs = useRef<Record<string, HTMLDivElement | null>>({})
  const navigableKeys = useMemo(
    () => selected.filter((key) => rowLoading[key] || rowErrors[key] || (rows[key]?.length ?? 0) > 0),
    [rowErrors, rowLoading, rows, selected],
  )
  const [activeKey, setActiveKey] = useState('')

  useEffect(() => {
    if (navigableKeys.length === 0) {
      setActiveKey('')
      return
    }

    const firstRow = navigableKeys.map((key) => rowRefs.current[key]).find(Boolean)
    const scrollContainer = firstRow?.closest('main')
    if (!scrollContainer) return

    let frame = 0
    const updateActiveKey = () => {
      const rail = visibleDiscoverJumpRail(scrollContainer)
      const activationLine = (rail
        ? discoverJumpAlignmentTop(rail)
        : scrollContainer.getBoundingClientRect().top + sectionTopOffset) + 1
      let nextKey = navigableKeys[0]
      for (const key of navigableKeys) {
        const row = rowRefs.current[key]
        if (!row) continue
        if (row.getBoundingClientRect().top <= activationLine) {
          nextKey = key
          continue
        }
        break
      }
      setActiveKey((current) => (current === nextKey ? current : nextKey))
    }
    const scheduleUpdate = () => {
      if (frame) return
      frame = window.requestAnimationFrame(() => {
        frame = 0
        updateActiveKey()
      })
    }

    updateActiveKey()
    scrollContainer.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('resize', scheduleUpdate)
    return () => {
      scrollContainer.removeEventListener('scroll', scheduleUpdate)
      window.removeEventListener('resize', scheduleUpdate)
      if (frame) window.cancelAnimationFrame(frame)
    }
  }, [navigableKeys])

  const jumpToSection = (key: string) => {
    const row = rowRefs.current[key]
    if (!row) return
    const scrollContainer = row.closest('main')
    if (!scrollContainer) return
    const rail = visibleDiscoverJumpRail(scrollContainer)
    const targetTop = scrollContainer.scrollTop
      + row.getBoundingClientRect().top
      - scrollContainer.getBoundingClientRect().top
      - sectionTopOffset
    setActiveKey(key)
    scrollContainer.scrollTop = Math.max(0, Math.round(targetTop))
    if (rail) {
      const correction = row.getBoundingClientRect().top - discoverJumpAlignmentTop(rail)
      if (Math.abs(correction) >= 1) {
        scrollContainer.scrollTop = Math.max(0, Math.round(scrollContainer.scrollTop + correction))
      }
    }
  }

  const jumpToTop = () => {
    const firstRow = navigableKeys.map((key) => rowRefs.current[key]).find(Boolean)
    const scrollContainer = firstRow?.closest('main')
    if (!scrollContainer) return
    scrollContainer.scrollTop = 0
  }

  return (
    <div>
      {navigableKeys.length > 1 && (
        <DiscoverMobileSectionJump
          keys={navigableKeys}
          activeKey={activeKey}
          sectionLabel={sectionLabel}
          onTop={jumpToTop}
          onSelect={jumpToSection}
        />
      )}

      <div className={navigableKeys.length > 1 ? 'xl:grid xl:grid-cols-[220px_minmax(0,1fr)] xl:gap-8' : ''}>
        {navigableKeys.length > 1 && (
          <DiscoverSectionRail
            keys={navigableKeys}
            activeKey={activeKey}
            sectionLabel={sectionLabel}
            onTop={jumpToTop}
            onSelect={jumpToSection}
          />
        )}

        <div className={`min-w-0 space-y-16 ${navigableKeys.length > 1 ? 'pb-[60vh]' : ''}`}>
          {selected.map((key, rowIndex) => {
            const rowItems = rows[key] ?? []
            const workRow = rowItems.some((item) => item.media_type !== 'person')
            const items = workRow ? rowItems.slice(0, discoverModulePageSize) : rowItems
            if (items.length === 0) {
              if (rowLoading[key]) {
                return (
                  <div
                    key={key}
                    ref={(element) => { rowRefs.current[key] = element }}
                    id={discoverSectionID(key)}
                    className="scroll-mt-16"
                  >
                    <DiscoverRowSkeleton
                      title={sectionLabel(key)}
                      personCards={discoverSectionUsesPersonCards(key)}
                      refreshStatus={rowRefreshStatus[key]}
                      onRefresh={() => onRefresh(key)}
                    />
                  </div>
                )
              }
              if (rowErrors[key]) {
                return (
                  <div
                    key={key}
                    ref={(element) => { rowRefs.current[key] = element }}
                    id={discoverSectionID(key)}
                    className="scroll-mt-16"
                  >
                    <DiscoverUnavailableRow
                      title={sectionLabel(key)}
                      message={rowErrors[key]}
                      refreshing={Boolean(rowLoading[key])}
                      refreshStatus={rowRefreshStatus[key]}
                      onRefresh={() => onRefresh(key)}
                    />
                  </div>
                )
              }
              return null
            }
            return (
              <div
                key={key}
                ref={(element) => { rowRefs.current[key] = element }}
                id={discoverSectionID(key)}
                className="scroll-mt-16"
              >
                <ContentRow
                  title={sectionLabel(key)}
                  items={items}
                  page={rowPages[key] ?? 1}
                  canNext={Boolean(rowCanNext[key])}
                  refreshing={Boolean(rowLoading[key])}
                  refreshStatus={rowRefreshStatus[key]}
                  fixedGrid={workRow}
                  priority={rowIndex === 0}
                  headerControl={key === 'adult_fd2ppv' ? (
                    <label className="inline-flex items-center gap-2 text-xs font-semibold text-sand-500">
                      <span className="hidden sm:inline">排序</span>
                      <select
                        aria-label="FC2 作品排序"
                        value={rowSorts[key] || 'release'}
                        disabled={Boolean(rowLoading[key] || rowSortSaving[key])}
                        onChange={(event) => onSortChange(key, event.target.value)}
                        className="h-8 rounded-lg border border-gray-200 bg-white px-2 text-xs font-semibold text-ink-600 outline-none transition focus:border-primary-300 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {fd2PPVSortOptions.map((option) => (
                          <option key={option.value} value={option.value}>{option.label}</option>
                        ))}
                      </select>
                    </label>
                  ) : undefined}
                  onPageChange={(delta) => onPageChange(key, delta)}
                  onRefresh={() => onRefresh(key)}
                  onSelect={onSelect}
                />
                {rowErrors[key] && <DiscoverRowWarning message={rowErrors[key]} />}
              </div>
            )
          })}

          {!loading && !hasContent && !hasRowErrors && <DiscoverNoContent />}
        </div>
      </div>
    </div>
  )
}

function DiscoverMobileSectionJump({
  keys,
  activeKey,
  sectionLabel,
  onTop,
  onSelect,
}: {
  keys: string[]
  activeKey: string
  sectionLabel: SectionLabel
  onTop: () => void
  onSelect: (key: string) => void
}) {
  return (
    <div
      data-discover-jump-rail
      data-discover-jump-mode="mobile"
      className="sticky -top-4 z-30 mb-4 flex items-center gap-2 rounded-xl border border-gray-200 bg-white/95 p-2 shadow-sm backdrop-blur xl:hidden"
    >
      <button
        type="button"
        onClick={onTop}
        className="inline-flex h-10 shrink-0 items-center gap-1.5 rounded-lg border border-gray-200 px-3 text-xs font-semibold text-gray-600"
      >
        <ArrowUp size={14} />
        顶部
      </button>
      <label className="relative min-w-0 flex-1">
        <List className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-sand-500" />
        <span className="sr-only">快速跳转模块</span>
        <select
          aria-label="移动端发现模块快速跳转"
          className="h-10 w-full rounded-lg border border-gray-200 bg-white pl-9 pr-3 text-sm font-semibold text-ink-600 outline-none"
          value={activeKey || keys[0]}
          onChange={(event) => onSelect(event.target.value)}
        >
          {keys.map((key) => <option key={key} value={key}>{sectionLabel(key)}</option>)}
        </select>
      </label>
    </div>
  )
}

function DiscoverSectionRail({
  keys,
  activeKey,
  sectionLabel,
  onTop,
  onSelect,
}: {
  keys: string[]
  activeKey: string
  sectionLabel: SectionLabel
  onTop: () => void
  onSelect: (key: string) => void
}) {
  return (
    <aside className="hidden xl:block">
      <nav
        data-discover-jump-rail
        aria-label="发现模块快速跳转"
        className="sticky -top-2 rounded-2xl border border-gray-200 bg-white/95 p-4 shadow-sm backdrop-blur"
      >
        <div className="flex items-center justify-between gap-2 text-xs font-semibold text-sand-500">
          <div className="flex items-center gap-2">
            <List size={14} />
            快速跳转
          </div>
          <button
            type="button"
            aria-label="回到顶部"
            title="回到顶部"
            onClick={onTop}
            className="inline-flex h-7 w-7 items-center justify-center rounded-lg text-gray-400 transition hover:bg-gray-50 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-300"
          >
            <ArrowUp size={15} />
          </button>
        </div>
        <div className="mt-3 border-l border-gray-200 pl-0.5">
          {keys.map((key) => {
            const active = key === activeKey
            return (
              <button
                key={key}
                type="button"
                aria-current={active ? 'true' : undefined}
                onClick={() => onSelect(key)}
                className={
                  'group -ml-[7px] flex w-[calc(100%+7px)] items-center gap-2 py-2 text-left text-xs font-semibold transition ' +
                  (active ? 'text-brand-500' : 'text-gray-500 hover:text-ink-600')
                }
              >
                <span
                  className={
                    'h-3 w-3 flex-none rounded-full border-2 bg-white transition ' +
                    (active
                      ? 'border-primary-500 ring-4 ring-primary-500/10'
                      : 'border-gray-300 group-hover:border-primary-300')
                  }
                />
                <span className="line-clamp-2">{sectionLabel(key)}</span>
              </button>
            )
          })}
        </div>
      </nav>
    </aside>
  )
}

function visibleDiscoverJumpRail(scrollContainer: Element): HTMLElement | undefined {
  return Array.from(scrollContainer.querySelectorAll<HTMLElement>('[data-discover-jump-rail]'))
    .find((element) => element.getClientRects().length > 0)
}

function discoverJumpAlignmentTop(rail: HTMLElement): number {
  const bounds = rail.getBoundingClientRect()
  return rail.dataset.discoverJumpMode === 'mobile' ? bounds.bottom + 12 : bounds.top
}

function discoverSectionID(key: string): string {
  return `discover-section-${key.replace(/[^a-zA-Z0-9_-]/g, '-')}`
}

function DiscoverUnavailableRow({
  title,
  message,
  refreshing,
  refreshStatus,
  onRefresh,
}: {
  title: string
  message: string
  refreshing: boolean
  refreshStatus?: DiscoverRefreshStatus
  onRefresh: () => void
}) {
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="pl-1 font-display text-2xl font-semibold text-ink-600">{title}</h2>
        <DiscoverRefreshControl
          title={title}
          refreshing={refreshing}
          status={refreshStatus}
          onRefresh={onRefresh}
        />
      </div>
      <DiscoverRowWarning message={message} />
    </section>
  )
}

function DiscoverRowWarning({ message }: { message: string }) {
  return (
    <div className="mt-3 flex items-start gap-3 rounded-lg border border-amber-300/70 bg-amber-50 px-3 py-2 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
      <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500" />
      <div className="text-xs">
        <p className="font-semibold">当前模块未获取到最新数据</p>
        <p className="mt-1">{message}</p>
      </div>
    </div>
  )
}

function DiscoverNoContent() {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-10 text-center">
      <p className="text-sand-500">
        当前选择的推荐源暂未返回内容，可切换豆瓣 / Bangumi 或检查网络代理。
      </p>
    </div>
  )
}

function DiscoverRowSkeleton({
  title,
  personCards,
  refreshStatus,
  onRefresh,
}: {
  title: string
  personCards: boolean
  refreshStatus?: DiscoverRefreshStatus
  onRefresh: () => void
}) {
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="pl-1 font-display text-2xl font-semibold text-ink-600">{title}</h2>
        {refreshStatus && (
          <DiscoverRefreshControl
            title={title}
            refreshing
            status={refreshStatus}
            onRefresh={onRefresh}
          />
        )}
      </div>
      <div className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
        {Array.from({ length: discoverModulePageSize }, (_, index) => index).map((item) => (
          <DiscoverCardPlaceholder key={item} person={personCards} pulse />
        ))}
      </div>
    </section>
  )
}

function discoverSectionUsesPersonCards(key: string): boolean {
  return key === 'adult_followed_performers' || key.startsWith('adult_javdb_performers')
}
