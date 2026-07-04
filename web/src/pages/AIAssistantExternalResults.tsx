import { useState } from 'react'

import type { ExternalMediaResult } from '../api/ai'
import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { DiscoverDetailModal } from './DiscoverDetailModal'

type AIAssistantExternalResultsProps = {
  items: ExternalMediaResult[]
}

export function AIAssistantExternalResults({ items }: AIAssistantExternalResultsProps) {
  const [activeItem, setActiveItem] = useState<ExternalMediaResult | null>(null)

  if (items.length === 0) return null

  return (
    <>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((item) => {
          const keyword = item.subscribe_keyword || item.title
          const key = `${item.source}:${keyword}`
          return (
            <article
              key={key}
              role="button"
              tabIndex={0}
              onClick={() => setActiveItem(item)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') setActiveItem(item)
              }}
              className="cursor-pointer rounded-2xl border border-gray-200 bg-gray-50 p-3 transition hover:-translate-y-0.5 hover:border-primary-300"
            >
              <div className="flex gap-3">
                <div className="h-24 w-16 shrink-0 overflow-hidden rounded-xl bg-white">
                  {item.poster_url ? (
                    <img
                      src={imageURL(item.poster_url)}
                      alt={item.title}
                      className="h-full w-full object-cover"
                    />
                  ) : null}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="mb-1 flex flex-wrap gap-2 text-[10px] uppercase text-brand-500">
                    <span>{item.source}</span>
                    {item.media_type && <span>{item.media_type}</span>}
                    {item.year ? <span>{item.year}</span> : null}
                  </div>
                  <h3 className="truncate font-semibold text-ink-600">{item.title}</h3>
                  <p className="mt-1 line-clamp-2 text-xs text-ink-50">
                    {item.overview || `订阅关键词：${keyword}`}
                  </p>
                  <p className="mt-2 text-xs font-semibold text-brand-500">详情 / 订阅设置</p>
                </div>
              </div>
            </article>
          )
        })}
      </div>
      {activeItem && (
        <DiscoverDetailModal
          item={activeItem as unknown as DiscoverItem}
          onClose={() => setActiveItem(null)}
        />
      )}
    </>
  )
}
