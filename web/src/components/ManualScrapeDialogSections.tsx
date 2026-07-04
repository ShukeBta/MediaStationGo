import { Check, LoaderCircle, Search, Sparkles, X } from 'lucide-react'

import { imageURL } from '../api/client'
import type { ManualScrapeCandidate } from '../api/library'
import {
  candidateIDText,
  candidateKey,
  manualSearchProviders,
  toggleProvider,
} from './ManualScrapeDialogModel'
import { EpisodeArtworkToggle } from './EpisodeArtworkToggle'

export function ManualScrapeDialogHeader({
  title,
  targetCount,
  onClose,
}: {
  title: string
  targetCount: number
  onClose: () => void
}) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-sand-200 px-5 py-4">
      <div>
        <h2 className="font-display text-xl font-bold text-ink-600">手动搜索刮削</h2>
        <p className="mt-1 text-xs text-sand-500">
          {title} · {targetCount > 1 ? `将应用到 ${targetCount} 个媒体` : '单个媒体'}
        </p>
      </div>
      <button onClick={onClose} className="btn-ghost h-9 w-9 p-0" aria-label="关闭">
        <X size={16} />
      </button>
    </div>
  )
}

interface ManualScrapeSearchControlsProps {
  query: string
  selectedProviders: string[]
  searching: boolean
  includeEpisodeArtwork: boolean
  showEpisodeArtworkToggle: boolean
  onQueryChange: (value: string) => void
  onProviderChange: (value: string[] | ((current: string[]) => string[])) => void
  onSearch: () => void
  onEpisodeArtworkChange: (checked: boolean) => void
}

export function ManualScrapeSearchControls({
  query,
  selectedProviders,
  searching,
  includeEpisodeArtwork,
  showEpisodeArtworkToggle,
  onQueryChange,
  onProviderChange,
  onSearch,
  onEpisodeArtworkChange,
}: ManualScrapeSearchControlsProps) {
  return (
    <div className="grid gap-4 border-b border-sand-200 bg-sand-50/40 p-5">
      <ManualScrapeQueryBar
        query={query}
        searching={searching}
        onQueryChange={onQueryChange}
        onSearch={onSearch}
      />
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-end">
        <ProviderSelector selectedProviders={selectedProviders} onProviderChange={onProviderChange} />
        {showEpisodeArtworkToggle && (
          <div className="flex justify-start xl:justify-end">
            <EpisodeArtworkToggle
              checked={includeEpisodeArtwork}
              onChange={onEpisodeArtworkChange}
              title="关闭后仍写入每集简介、评分和时长，只跳过每集图片"
            />
          </div>
        )}
      </div>
    </div>
  )
}

function ProviderSelector({
  selectedProviders,
  onProviderChange,
}: {
  selectedProviders: string[]
  onProviderChange: (value: string[] | ((current: string[]) => string[])) => void
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <span className="text-xs font-bold text-sand-500">刮削源</span>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:flex lg:flex-wrap">
        <button
          type="button"
          aria-pressed={selectedProviders.length === 0}
          onClick={() => onProviderChange([])}
          className={providerButtonClass(selectedProviders.length === 0)}
        >
          {selectedProviders.length === 0 && <Check size={13} />}
          全部源
        </button>
        {manualSearchProviders.map((item) => {
          const active = selectedProviders.includes(item.value)
          return (
            <button
              key={item.value}
              type="button"
              aria-pressed={active}
              onClick={() => onProviderChange((current) => toggleProvider(current, item.value))}
              className={providerButtonClass(active)}
            >
              {active && <Check size={13} />}
              {item.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function ManualScrapeQueryBar({
  query,
  searching,
  onQueryChange,
  onSearch,
}: {
  query: string
  searching: boolean
  onQueryChange: (value: string) => void
  onSearch: () => void
}) {
  return (
    <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_7.5rem]">
      <div className="relative min-w-0">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-sand-500" />
        <input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onKeyDown={(event) => { if (event.key === 'Enter') onSearch() }}
          className="h-11 w-full rounded-xl border border-sand-200 bg-white pl-9 pr-3 text-sm font-semibold text-ink-600 outline-none transition focus:border-brand-300 focus:ring-2 focus:ring-brand-100"
          placeholder="输入标题或 TMDb / 豆瓣 / Bangumi / TheTVDB ID"
        />
      </div>
      <button type="button" onClick={onSearch} disabled={searching} className="btn-primary h-11 justify-center px-5">
        {searching ? <LoaderCircle size={16} className="animate-spin" /> : <Sparkles size={16} />}
        搜索
      </button>
    </div>
  )
}

export function ManualScrapeCandidateList({
  items,
  applyingKey,
  onApply,
}: {
  items: ManualScrapeCandidate[]
  applyingKey: string
  onApply: (item: ManualScrapeCandidate) => void
}) {
  if (items.length === 0) {
    return (
      <div className="flex min-h-56 items-center justify-center rounded-xl border border-dashed border-sand-200 text-sm text-sand-500">
        搜索后在这里选择正确的元数据
      </div>
    )
  }

  return (
    <div className="grid gap-3">
      {items.map((item) => {
        const key = candidateKey(item)
        return (
          <ManualScrapeCandidateRow
            key={key}
            item={item}
            applying={applyingKey === key}
            disabled={!!applyingKey}
            onApply={onApply}
          />
        )
      })}
    </div>
  )
}

function ManualScrapeCandidateRow({
  item,
  applying,
  disabled,
  onApply,
}: {
  item: ManualScrapeCandidate
  applying: boolean
  disabled: boolean
  onApply: (item: ManualScrapeCandidate) => void
}) {
  return (
    <div className="flex flex-col gap-4 rounded-xl border border-sand-200 bg-white p-3 shadow-sm sm:flex-row">
      <div className="h-28 w-20 shrink-0 overflow-hidden rounded-lg bg-sand-100">
        {item.poster_url ? (
          <img src={imageURL(item.poster_url)} alt={item.title} className="h-full w-full object-cover" referrerPolicy="no-referrer" />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-sand-500">无海报</div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="truncate font-semibold text-ink-600">{item.title}</h3>
          <span className="rounded-full bg-brand-50 px-2 py-0.5 text-[11px] font-bold uppercase text-brand-700">{item.source}</span>
          {item.nsfw ? <span className="rounded-full bg-rose-50 px-2 py-0.5 text-[11px] font-bold text-rose-600">成人</span> : null}
          {item.year ? <span className="text-xs text-sand-500">{item.year}</span> : null}
        </div>
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-ink-50">{item.overview || '暂无简介'}</p>
        <p className="mt-2 text-[11px] font-semibold text-sand-500">{candidateIDText(item)}</p>
      </div>
      <button onClick={() => onApply(item)} disabled={disabled} className="btn-outline h-10 w-full shrink-0 justify-center px-3 text-xs sm:w-auto sm:self-center">
        {applying ? <LoaderCircle size={14} className="animate-spin" /> : <Check size={14} />}
        应用匹配
      </button>
    </div>
  )
}

function providerButtonClass(active: boolean): string {
  return (
    'inline-flex h-9 min-w-0 shrink-0 items-center justify-center gap-1.5 rounded-lg border px-3 text-xs font-bold transition ' +
    (active
      ? 'border-brand-300 bg-brand-50 text-brand-700'
      : 'border-sand-200 bg-white text-sand-600 hover:border-brand-200 hover:text-brand-600')
  )
}
