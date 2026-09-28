import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Clock, Play, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { historyAPI } from '../api/history'
import { imageURL } from '../api/client'
import { confirmAction } from '../components/confirmAction'
import { Pagination } from '../components/Pagination'
import type { HistoryItem } from '../types'
import { mediaPosterURL } from '../utils/mediaArtwork'

function fmtDuration(ms: number): string {
  if (!ms || ms <= 0) return '—'
  const sec = Math.floor(ms / 1000)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

// WatchHistoryPage shows a full paginated view of the user's playback
// history with progress bars and resume links.
export function WatchHistoryPage() {
  const pageSize = 20
  const [items, setItems] = useState<HistoryItem[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    historyAPI
      .listPage(page, pageSize)
      .then((result) => {
        setItems(result.items)
        setTotal(result.total)
      })
      .finally(() => setLoading(false))
  }, [page])

  useEffect(() => { load() }, [load])

  const removeOne = async (id: string) => {
    if (!(await confirmAction({ title: '移除观看历史', message: '确定移除此条观看历史？不会删除媒体文件。', confirmText: '移除' }))) return
    setBusy(id)
    try {
      await historyAPI.remove(id)
      setItems((prev) => prev.filter((item) => item.id !== id))
      setTotal((current) => Math.max(0, current - 1))
      if (items.length === 1 && page > 1) setPage((current) => current - 1)
      toast.success('已移除观看历史')
    } finally {
      setBusy('')
    }
  }

  const clearByStatus = async (status: 'completed' | 'incomplete') => {
    const label = status === 'completed' ? '已看完' : '未看完'
    if (!(await confirmAction({ title: '清除观看历史', message: `确定清除所有${label}的观看历史？不会删除媒体文件。`, confirmText: '清除' }))) return
    setBusy(status)
    try {
      await historyAPI.clear(undefined, status)
      if (page === 1) load()
      else setPage(1)
      toast.success(`已清除${label}记录`)
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Clock className="h-6 w-6 text-brand-500" />
          <h1 className="font-display text-3xl font-bold text-ink-600">观看历史</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => clearByStatus('incomplete')}
            disabled={busy !== ''}
            className="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-600 shadow-sm transition hover:border-amber-300 hover:text-amber-600 disabled:cursor-not-allowed disabled:opacity-45"
          >
            清除未看完
          </button>
          <button
            onClick={() => clearByStatus('completed')}
            disabled={busy !== ''}
            className="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-600 shadow-sm transition hover:border-emerald-300 hover:text-emerald-600 disabled:cursor-not-allowed disabled:opacity-45"
          >
            清除已看完
          </button>
        </div>
      </header>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && items.length === 0 && (
        <p className="text-ink-50">暂无观看记录。</p>
      )}

      <div className="space-y-3">
        {items.map((h) => {
          const m = h.media
          if (!m) return null
          const poster = mediaPosterURL(m)
          const progress =
            h.duration_ms > 0 ? h.position_ms / h.duration_ms : 0
          return (
            <div
              key={h.id}
              className="glass-panel flex items-center gap-4 !p-3"
            >
              <div className="h-16 w-12 shrink-0 overflow-hidden rounded-lg bg-surface-900">
                {poster ? (
                  <img
                    src={imageURL(poster, m.updated_at, { maxWidth: 96, quality: 78 })}
                    alt={m.title}
                    loading="lazy"
                    decoding="async"
                    className="h-full w-full object-cover"
                    referrerPolicy="no-referrer"
                  />
                ) : null}
              </div>
              <div className="flex-1 space-y-1">
                <Link
                  to={`/media/${m.id}`}
                  className="font-medium text-ink-600 transition hover:text-brand-500"
                >
                  {m.title}
                </Link>
                <div className="flex items-center gap-3 text-xs text-ink-50">
                  <span>{fmtDuration(h.position_ms)} / {fmtDuration(h.duration_ms)}</span>
                  <span>{new Date(h.watched_at).toLocaleString()}</span>
                  {h.completed && (
                    <span className="rounded-lg border border-emerald-400/40 px-1.5 py-0.5 text-emerald-400">
                      已看完
                    </span>
                  )}
                </div>
                <div className="h-1 w-full overflow-hidden rounded-lg bg-gray-200">
                  <div
                    className="h-full bg-primary-400"
                    style={{ width: `${Math.round(progress * 100)}%` }}
                  />
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Link
                  to={`/play/${m.id}`}
                  className="neon-button !px-3 !py-1 !text-xs"
                >
                  <Play size={12} /> 继续
                </Link>
                <button
                  onClick={() => removeOne(h.id)}
                  disabled={busy === h.id}
                  className="inline-flex items-center gap-1 rounded-xl border border-gray-200 bg-white px-3 py-1.5 text-xs font-bold text-gray-500 transition hover:border-red-200 hover:bg-red-50 hover:text-red-500 disabled:opacity-50"
                  title="移除此条观看历史"
                >
                  <Trash2 size={12} />
                  移除
                </button>
              </div>
            </div>
          )
        })}
      </div>

      {Math.ceil(total / pageSize) > 1 && (
        <Pagination
          page={page}
          totalPages={Math.ceil(total / pageSize)}
          onPageChange={setPage}
          className="pt-2"
        />
      )}
    </div>
  )
}
