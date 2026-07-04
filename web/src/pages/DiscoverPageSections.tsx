import { AlertTriangle, RefreshCw, Sparkles } from 'lucide-react'

import type { DiscoverItem, DiscoverSection } from '../api/discover'
import { ContentRow } from './DiscoverContentRow'

type SectionLabel = (key: string) => string

export function DiscoverHeader({
  sections,
  selected,
  sectionsReady,
  loading,
  onRefresh,
  onToggleSection,
}: {
  sections: DiscoverSection[]
  selected: string[]
  sectionsReady: boolean
  loading: boolean
  onRefresh: () => void
  onToggleSection: (key: string) => void
}) {
  return (
    <header className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
      <div className="flex items-center gap-4">
        <div className="rounded-2xl border border-primary-500/20 bg-gradient-to-br from-primary-500/20 to-primary-600/10 p-3">
          <Sparkles className="h-8 w-8 text-brand-500" />
        </div>
        <div>
          <h1 className="font-display text-4xl font-bold tracking-tight text-ink-600">
            发现
          </h1>
          <p className="mt-1 text-base text-ink-50">
            多源推荐：TMDb / 豆瓣 / Bangumi，可按需组合显示
          </p>
        </div>
      </div>

      <div className="flex flex-col gap-3 lg:items-end">
        <button
          type="button"
          onClick={onRefresh}
          disabled={!sectionsReady || selected.length === 0}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-semibold text-ink-600 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
        <div className="flex flex-wrap justify-start gap-2 lg:justify-end">
          {sections.map((section) => {
            const active = selected.includes(section.key)
            return (
              <button
                key={section.key}
                type="button"
                onClick={() => onToggleSection(section.key)}
                className={
                  'rounded-full border px-3 py-1.5 text-xs font-semibold transition ' +
                  (active
                    ? 'border-primary-400 bg-primary-400/15 text-brand-500'
                    : 'border-gray-200 bg-white text-gray-500 hover:border-primary-300 hover:text-ink-600')
                }
              >
                {section.label}
              </button>
            )
          })}
        </div>
      </div>
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
  rowPages,
  rowCanNext,
  loading,
  hasContent,
  imageVersion,
  refreshImageVersion,
  sectionLabel,
  onPageChange,
  onSelect,
}: {
  selected: string[]
  rows: Record<string, DiscoverItem[]>
  rowLoading: Record<string, boolean>
  rowErrors: Record<string, string>
  rowPages: Record<string, number>
  rowCanNext: Record<string, boolean>
  loading: boolean
  hasContent: boolean
  imageVersion: string
  refreshImageVersion?: string
  sectionLabel: SectionLabel
  onPageChange: (key: string, delta: number) => void
  onSelect: (item: DiscoverItem) => void
}) {
  const hasRowErrors = Object.keys(rowErrors).length > 0

  return (
    <div className="space-y-10">
      {selected.map((key) => {
        const items = rows[key] ?? []
        if (items.length === 0) {
          if (rowLoading[key]) {
            return <DiscoverRowSkeleton key={key} title={sectionLabel(key)} />
          }
          return null
        }
        return (
          <ContentRow
            key={key}
            title={sectionLabel(key)}
            items={items}
            page={rowPages[key] ?? 1}
            canNext={Boolean(rowCanNext[key])}
            imageVersion={imageVersion}
            refreshImageVersion={refreshImageVersion}
            onPageChange={(delta) => onPageChange(key, delta)}
            onSelect={onSelect}
          />
        )
      })}

      {hasRowErrors && (
        <DiscoverRowErrors rowErrors={rowErrors} sectionLabel={sectionLabel} />
      )}

      {!loading && !hasContent && !hasRowErrors && <DiscoverNoContent />}
    </div>
  )
}

function DiscoverRowErrors({
  rowErrors,
  sectionLabel,
}: {
  rowErrors: Record<string, string>
  sectionLabel: SectionLabel
}) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-amber-300/70 bg-amber-50 px-3 py-2 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
      <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500" />
      <div className="space-y-1 text-xs">
        <p className="font-semibold">部分推荐源暂不可用，其他已加载内容不受影响。</p>
        {Object.entries(rowErrors).map(([key, message]) => (
          <p key={key}>{sectionLabel(key)}：{message}</p>
        ))}
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

function DiscoverRowSkeleton({ title }: { title: string }) {
  return (
    <section className="space-y-4">
      <h2 className="pl-1 font-display text-2xl font-semibold text-ink-600">{title}</h2>
      <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-7 xl:grid-cols-8">
        {[1, 2, 3, 4, 5, 6, 7, 8].map((item) => (
          <div key={item} className="aspect-[2/3] animate-pulse rounded-xl bg-gray-100" />
        ))}
      </div>
    </section>
  )
}
