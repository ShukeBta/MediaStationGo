import { ChevronLeft, ChevronRight, Loader2, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { adminAPI, type AdminUserHistoryPage } from '../api/admin'
import type { User } from '../types'

const pageSize = 20

export function AdminUserHistoryModal({ user, onClose }: { user: User; onClose: () => void }) {
  const [page, setPage] = useState(1)
  const [data, setData] = useState<AdminUserHistoryPage | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    setLoading(true)
    adminAPI
      .listUserHistory(user.id, page, pageSize)
      .then((result) => {
        if (active) setData(result)
      })
      .catch((err: unknown) => {
        if (!active) return
        const message =
          (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          '加载播放记录失败'
        toast.error(message)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [page, user.id])

  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4" role="dialog" aria-modal="true">
      <section className="flex max-h-[82vh] w-full max-w-3xl flex-col overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-2xl">
        <header className="flex shrink-0 items-center justify-between border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="font-display text-lg font-semibold text-ink-600">{user.username} 的播放记录</h2>
            <p className="mt-1 text-xs text-ink-50">共 {data?.total ?? 0} 条数据库记录</p>
          </div>
          <button className="rounded-xl border border-gray-200 p-2 text-ink-50 hover:bg-gray-100" onClick={onClose} title="关闭">
            <X size={16} />
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {loading ? (
            <div className="flex min-h-40 items-center justify-center text-ink-50"><Loader2 size={20} className="animate-spin" /></div>
          ) : (data?.items.length ?? 0) === 0 ? (
            <p className="py-12 text-center text-sm text-ink-50">暂无播放记录</p>
          ) : (
            <div className="space-y-2">
              {data?.items.map((item) => {
                const progress = item.duration_ms > 0 ? Math.min(100, Math.round((item.position_ms / item.duration_ms) * 100)) : 0
                return (
                  <div key={item.id} className="rounded-2xl border border-gray-200 bg-gray-50/70 px-4 py-3">
                    <div className="flex items-start justify-between gap-4">
                      <div className="min-w-0">
                        <p className="truncate font-medium text-ink-600">{item.media?.title ?? '媒体已不存在'}</p>
                        <p className="mt-1 text-xs text-ink-50">
                          {formatDuration(item.position_ms)} / {formatDuration(item.duration_ms)} · {new Date(item.watched_at).toLocaleString()}
                        </p>
                      </div>
                      <span className={item.completed ? 'shrink-0 text-xs text-emerald-500' : 'shrink-0 text-xs text-ink-50'}>
                        {item.completed ? '已看完' : `${progress}%`}
                      </span>
                    </div>
                    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-200">
                      <div className="h-full bg-primary-400" style={{ width: `${progress}%` }} />
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        <footer className="flex shrink-0 items-center justify-between border-t border-gray-200 px-5 py-3 text-sm text-ink-50">
          <span>第 {page} / {totalPages} 页</span>
          <div className="flex gap-2">
            <button className="rounded-lg border border-gray-200 p-2 disabled:opacity-35" disabled={loading || page <= 1} onClick={() => setPage((value) => value - 1)} title="上一页">
              <ChevronLeft size={15} />
            </button>
            <button className="rounded-lg border border-gray-200 p-2 disabled:opacity-35" disabled={loading || page >= totalPages} onClick={() => setPage((value) => value + 1)} title="下一页">
              <ChevronRight size={15} />
            </button>
          </div>
        </footer>
      </section>
    </div>
  )
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = seconds % 60
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
    : `${minutes}:${String(rest).padStart(2, '0')}`
}
