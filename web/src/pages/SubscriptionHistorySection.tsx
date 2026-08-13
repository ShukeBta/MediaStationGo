import { AlertTriangle, Archive, Film, Play, RefreshCw, RotateCcw } from 'lucide-react'

import { imageURL } from '../api/client'
import type { Subscription } from '../types'
import { subscriptionProgressLabel } from './subscriptionPageModel'

interface SubscriptionHistorySectionProps {
  subscriptions: Subscription[]
  loading?: boolean
  error?: string
  onRefresh?: () => Promise<void>
  onRestore: (subscription: Subscription, runAfterRestore?: boolean) => void
}

export function SubscriptionHistorySection({
  subscriptions,
  loading = false,
  error = '',
  onRefresh,
  onRestore,
}: SubscriptionHistorySectionProps) {
  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Archive size={18} className="text-brand-500" />
          <h2 className="font-display text-xl font-semibold text-ink-600">订阅历史</h2>
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-ink-50">{subscriptions.length} 条</span>
        </div>
        {onRefresh && (
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-ink-100 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => onRefresh().catch(() => undefined)}
            disabled={loading}
          >
            <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
            刷新历史
          </button>
        )}
      </div>

      {loading && <p className="text-sm text-sand-500">历史加载中…</p>}
      {!loading && error && (
        <div className="flex items-center gap-2 rounded-2xl border border-red-300/70 bg-red-50 px-4 py-3 text-sm text-red-700">
          <AlertTriangle size={16} className="shrink-0" />
          <span className="break-words">订阅历史加载失败：{error}</span>
        </div>
      )}
      {!loading && !error && subscriptions.length === 0 && <p className="text-sm text-ink-50">暂无历史订阅。</p>}

      {!error && subscriptions.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {subscriptions.map((subscription) => (
          <article key={subscription.id} className="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm">
            <div className="flex gap-3">
              <div className="h-20 w-14 flex-shrink-0 overflow-hidden rounded-xl bg-primary-400/10">
                {subscription.poster_url ? (
                  <img
                    src={imageURL(subscription.poster_url, subscription.updated_at, { maxWidth: 128, quality: 80 })}
                    alt={subscription.name}
                    loading="lazy"
                    decoding="async"
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <div className="flex h-full items-center justify-center text-brand-500">
                    <Film size={18} />
                  </div>
                )}
              </div>
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold text-ink-600" title={subscription.name}>
                  {subscription.name}
                </h3>
                <p className="mt-1 text-xs text-ink-50">{subscription.archive_reason || '订阅已完成'}</p>
                <p className="mt-2 text-xs text-ink-100">
                  {subscription.archived_at ? new Date(subscription.archived_at).toLocaleString() : '完成时间未知'}
                </p>
                <p className="mt-1 text-xs text-ink-50">{subscriptionProgressLabel(subscription)}</p>
                {subscription.import_jobs && subscription.import_jobs.length > 0 && (
                  <div className="mt-3 space-y-1 border-t border-gray-100 pt-2">
                    <p className="text-xs font-semibold text-ink-100">追更任务记录</p>
                    {subscription.import_jobs.slice(0, 5).map((job) => (
                      <div key={job.id} className="text-xs text-ink-50">
                        <span className="font-medium text-ink-100">
                          第 {job.attempt || 1} 次 · {subscriptionImportOutcomeLabel(job.outcome, job.status)}
                        </span>
                        {job.candidate_title ? ` · ${job.candidate_title}` : ''}
                        {job.error ? <p className="break-words text-red-500">{job.error}</p> : null}
                      </div>
                    ))}
                  </div>
                )}
                <div className="mt-3 flex flex-wrap gap-2">
                  <button
                    className="rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-ink-100 hover:bg-gray-50"
                    onClick={() => onRestore(subscription)}
                  >
                    <RotateCcw size={13} className="mr-1 inline" />
                    恢复订阅
                  </button>
                  <button
                    className="rounded-xl border border-primary-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-primary-400/10"
                    onClick={() => onRestore(subscription, true)}
                  >
                    <Play size={13} className="mr-1 inline" />
                    恢复并运行
                  </button>
                </div>
              </div>
            </div>
          </article>
        ))}
        </div>
      )}
    </section>
  )
}

function subscriptionImportOutcomeLabel(outcome = '', status = ''): string {
  const labels: Record<string, string> = {
    imported: '已入库',
    no_new_episodes: '无新增集',
    rejected: '已拒绝',
    failed: '失败',
    superseded: '已替代',
    canceled: '已取消',
  }
  return labels[outcome] || (status === 'completed' ? '已完成' : status || '处理中')
}
