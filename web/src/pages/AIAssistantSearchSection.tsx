import type { FormEvent } from 'react'
import { Loader2, Search } from 'lucide-react'

import type { ExternalMediaResult, SearchIntent } from '../api/ai'
import { MediaCard } from '../components/MediaCard'
import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'
import { AIAssistantExternalResults } from './AIAssistantExternalResults'

type AIAssistantSearchSectionProps = {
  query: string
  searching: boolean
  intent: SearchIntent | null
  items: Media[]
  localCards: SeriesCard[]
  externalItems: ExternalMediaResult[]
  onSearch: (event: FormEvent) => void
  setQuery: (query: string) => void
}

const quickHints = [
  '2023 年的科幻电影',
  '评分高的动漫',
  '最近添加的纪录片',
  '中文剧集',
]

export function AIAssistantSearchSection({
  query,
  searching,
  intent,
  items,
  localCards,
  externalItems,
  onSearch,
  setQuery,
}: AIAssistantSearchSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <h2 className="font-display text-lg font-semibold text-ink-600">智能搜索</h2>
      <form onSubmit={onSearch} className="flex flex-wrap gap-2">
        <input
          className="input-base flex-1"
          placeholder="试试: 2023 年的高分动作片"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button type="submit" disabled={searching} className="neon-button">
          {searching ? <Loader2 size={16} className="animate-spin" /> : <Search size={16} />}
          搜索
        </button>
      </form>

      <div className="flex flex-wrap gap-2">
        {quickHints.map((hint) => (
          <button
            key={hint}
            onClick={() => setQuery(hint)}
            className="rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
          >
            {hint}
          </button>
        ))}
      </div>

      <SearchIntentSummary intent={intent} />
      <LocalMediaResults localCards={localCards} itemCount={items.length} />
      <AIAssistantExternalResults items={externalItems} />
    </section>
  )
}

function SearchIntentSummary({ intent }: { intent: SearchIntent | null }) {
  if (!intent) return null
  return (
    <div className="rounded-xl border border-gray-200 bg-gray-50 p-3 text-xs text-ink-100">
      <div className="mb-1 font-semibold text-ink-200">解析结果</div>
      <div className="flex flex-wrap gap-x-6 gap-y-1">
        <span>
          查询: <span className="text-brand-500">{intent.query || '—'}</span>
        </span>
        {intent.year !== undefined && intent.year > 0 && (
          <span>
            年份: <span className="text-brand-500">{intent.year}</span>
          </span>
        )}
        {intent.genre && (
          <span>
            类型: <span className="text-brand-500">{intent.genre}</span>
          </span>
        )}
        {intent.type && (
          <span>
            分类: <span className="text-brand-500">{intent.type}</span>
          </span>
        )}
        {intent.sort && (
          <span>
            排序: <span className="text-brand-500">{intent.sort}</span>
          </span>
        )}
        {intent.language && (
          <span>
            语言: <span className="text-brand-500">{intent.language}</span>
          </span>
        )}
      </div>
    </div>
  )
}

function LocalMediaResults({
  localCards,
  itemCount,
}: {
  localCards: SeriesCard[]
  itemCount: number
}) {
  if (localCards.length === 0) return null
  return (
    <div className="space-y-2">
      <div className="text-sm font-semibold text-ink-100">
        本地媒体库 · {localCards.length} 个合集 / {itemCount} 个条目
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        {localCards.map((card) => (
          <MediaCard
            key={card.key}
            media={card.rep}
            count={card.count}
            linkTo={seriesCardLink(card)}
          />
        ))}
      </div>
    </div>
  )
}
