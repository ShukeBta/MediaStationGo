import type { ReactNode } from 'react'
import { ArrowDown, ArrowUp, Film, HardDrive, Rss, Trash2 } from 'lucide-react'

import { imageURL } from '../api/client'
import type { DownloadCardItem } from './downloadTaskCardModel'

export function DownloadTaskCard({
  item,
  removable,
  onRemove,
}: {
  item: DownloadCardItem
  removable?: boolean
  onRemove?: () => Promise<void>
}) {
  const progress = pct(item.progress)
  const visual = item.poster_url || item.backdrop_url
  const downloaded = item.downloaded || (item.size ? Math.round(item.size * (item.progress || 0)) : 0)

  return (
    <article className="group overflow-hidden rounded-3xl border border-white/70 bg-white shadow-sm transition hover:-translate-y-1 hover:shadow-xl">
      <div className="relative flex gap-4 p-4">
        <div className="relative h-40 w-28 flex-shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-400/15 via-white to-surface-200 shadow-inner">
          {visual ? (
            <img
              src={imageURL(visual, item.updated_at)}
              alt={item.title}
              className="h-full w-full object-cover"
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="flex h-full w-full flex-col items-center justify-center gap-2 px-3 text-center text-xs font-semibold text-brand-500">
              <Film size={24} />
              <span className="line-clamp-3">{item.title}</span>
            </div>
          )}
          <span className={`absolute left-2 top-2 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone(item)}`}>
            {stateLabel(item)}
          </span>
        </div>

        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <h2 className="line-clamp-2 font-display text-lg font-semibold leading-snug text-ink-600" title={item.title}>
              {item.title}
            </h2>
            <p className="mt-1 line-clamp-2 text-xs leading-5 text-ink-50">
              {item.overview || item.save_path || '已隐藏原始种子 URL，避免泄露私有 Token。'}
            </p>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between text-xs font-semibold text-ink-100">
              <span>进度 {progress.toFixed(1)}%</span>
              <span>{fmtBytes(downloaded)} / {fmtBytes(item.size)}</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-gray-100">
              <div
                className="h-full rounded-full bg-gradient-to-r from-primary-400 to-brand-500 transition-all"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2 text-xs text-ink-100">
            <DownloadMetric icon={<ArrowDown size={13} className="mr-1 inline text-brand-500" />} value={fmtSpeed(item.dlspeed)} />
            <DownloadMetric icon={<ArrowUp size={13} className="mr-1 inline text-brand-500" />} value={fmtSpeed(item.upspeed)} />
            <DownloadMetric icon={<Rss size={13} className="mr-1 inline text-brand-500" />} value={`${item.num_seeds ?? 0} / ${item.num_leechs ?? 0}`} />
            <DownloadMetric icon={<HardDrive size={13} className="mr-1 inline text-brand-500" />} value={fmtBytes(item.size)} />
          </div>

          <div className="flex items-center justify-between gap-2 text-xs text-ink-50">
            <span className="truncate" title={item.save_path || ''}>
              {item.save_path || '默认下载目录'}
            </span>
            {item.created_at && <span>{new Date(item.created_at).toLocaleString()}</span>}
          </div>
        </div>
      </div>

      {removable && onRemove && (
        <div className="flex justify-end border-t border-gray-100 bg-gray-50/70 px-4 py-3">
          <button
            className="rounded-xl border border-red-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-red-400 hover:bg-red-400/10"
            onClick={() => void onRemove()}
          >
            <Trash2 size={13} className="mr-1 inline" />
            删除任务
          </button>
        </div>
      )}
    </article>
  )
}

function DownloadMetric({ icon, value }: { icon: ReactNode; value: string }) {
  return (
    <div className="rounded-2xl bg-gray-50 px-3 py-2">
      {icon}
      {value}
    </div>
  )
}

function fmtBytes(n?: number): string {
  if (!n || n <= 0) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${u[i]}`
}

function fmtSpeed(n?: number): string {
  return `${fmtBytes(n)}/s`
}

function pct(progress?: number): number {
  if (!Number.isFinite(progress)) return 0
  return Math.min(100, Math.max(0, Math.round((progress ?? 0) * 1000) / 10))
}

function stateLabel(item: DownloadCardItem): string {
  const state = (item.state || item.status || 'queued').toLowerCase()
  if (state.includes('down') || state.includes('meta')) return '下载中'
  if (state.includes('up') || state.includes('seed')) return '做种中'
  if (state.includes('pause')) return '已暂停'
  if (state.includes('error')) return '出错'
  if (state.includes('complete') || pct(item.progress) >= 100) return '已完成'
  if (state.includes('queue')) return '排队中'
  return item.state || item.status || '等待中'
}

function statusTone(item: DownloadCardItem): string {
  const state = stateLabel(item)
  if (state === '已完成' || state === '做种中') return 'bg-emerald-50 text-emerald-600'
  if (state === '出错') return 'bg-red-50 text-red-500'
  if (state === '已暂停') return 'bg-amber-50 text-amber-600'
  return 'bg-primary-400/10 text-brand-500'
}
