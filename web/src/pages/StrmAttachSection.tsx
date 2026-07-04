import type { FormEvent } from 'react'
import { Search, Trash2 } from 'lucide-react'

import type { Media } from '../types'

type StrmAttachSectionProps = {
  query: string
  searching: boolean
  results: Media[]
  drafts: Record<string, string>
  doSearch: (event?: FormEvent) => void
  setQuery: (value: string) => void
  setDrafts: (updater: (drafts: Record<string, string>) => Record<string, string>) => void
  onAttach: (media: Media) => void
  onDetach: (media: Media) => void
}

export function StrmAttachSection({
  query,
  searching,
  results,
  drafts,
  doSearch,
  setQuery,
  setDrafts,
  onAttach,
  onDetach,
}: StrmAttachSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <h2 className="font-display text-lg font-semibold text-ink-600">附加 STRM URL 到已有媒体</h2>
      <form onSubmit={doSearch} className="flex flex-wrap gap-2">
        <input
          className="input-base flex-1"
          placeholder="搜索媒体标题…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button type="submit" disabled={searching} className="neon-button">
          <Search size={16} /> {searching ? '搜索中…' : '搜索'}
        </button>
      </form>

      {results.length > 0 && (
        <div className="space-y-3">
          {results.map((media) => {
            const isStrm = media.container === 'strm'
            return (
              <div
                key={media.id}
                className="space-y-3 rounded-xl border border-gray-200 bg-gray-50 p-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate font-medium text-ink-600">{media.title}</div>
                    <div className="truncate text-xs text-ink-50">
                      {media.year > 0 && `${media.year} · `}
                      {media.container || '本地文件'}
                    </div>
                    {isStrm && (
                      <div className="mt-1 break-all rounded-lg bg-emerald-400/10 px-2 py-0.5 text-xs text-emerald-300">
                        已设置 STRM
                      </div>
                    )}
                  </div>
                  {isStrm && (
                    <button
                      onClick={() => onDetach(media)}
                      className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                    >
                      <Trash2 size={12} className="inline" /> 清除
                    </button>
                  )}
                </div>
                <div className="flex flex-wrap gap-2">
                  <input
                    className="input-base flex-1"
                    placeholder="https://example.com/stream.m3u8"
                    value={drafts[media.id] ?? ''}
                    onChange={(e) =>
                      setDrafts((current) => ({ ...current, [media.id]: e.target.value }))
                    }
                  />
                  <button
                    onClick={() => onAttach(media)}
                    disabled={!(drafts[media.id] ?? '').trim()}
                    className="neon-button"
                  >
                    {isStrm ? '替换 URL' : '设置'}
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {!searching && query && results.length === 0 && (
        <p className="text-sm text-ink-50">未找到匹配的媒体。</p>
      )}
    </section>
  )
}
