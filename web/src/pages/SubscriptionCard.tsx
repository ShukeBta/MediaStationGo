import { useState } from 'react'
import { Link } from 'react-router-dom'
import { CalendarClock, CheckCircle2, Film, Pause, Pencil, Play, Power, ShieldCheck, Trash2 } from 'lucide-react'

import { imageURL } from '../api/client'
import type { Subscription } from '../types'
import { subscriptionProgressLabel, subscriptionRuleBadges } from './subscriptionPageModel'

interface SubscriptionCardProps {
  subscription: Subscription
  onEdit: (subscription: Subscription) => void
  onSetEnabled: (subscription: Subscription, enabled: boolean) => void
  onRunNow: (subscription: Subscription) => void
  onRemove: (subscription: Subscription) => void
}

export function SubscriptionCard({ subscription, onEdit, onSetEnabled, onRunNow, onRemove }: SubscriptionCardProps) {
	const importJobs = subscription.import_jobs || []
	const [importPage, setImportPage] = useState(0)
	const importPageSize = 5
	const importPageCount = Math.ceil(importJobs.length / importPageSize)
	const visibleImportPage = Math.min(importPage, Math.max(0, importPageCount - 1))
	const visibleImportJobs = importJobs.slice(visibleImportPage * importPageSize, (visibleImportPage + 1) * importPageSize)
	const poster = (
		<>
			{subscription.poster_url ? (
				<img
					src={imageURL(subscription.poster_url, subscription.updated_at, { maxWidth: 240, quality: 82 })}
					alt={subscription.name}
					loading="lazy"
					decoding="async"
					className="h-full w-full object-cover"
				/>
			) : (
				<div className="flex h-full w-full flex-col items-center justify-center gap-2 px-2 text-center text-xs font-semibold text-brand-500">
					<Film size={22} />
					<span className="line-clamp-3">{subscription.name}</span>
				</div>
			)}
		</>
	)

  return (
    <article className="group overflow-hidden rounded-3xl border border-white/70 bg-white shadow-sm transition hover:-translate-y-1 hover:shadow-xl">
      <div className="relative flex gap-4 p-4">
		{subscription.media_id ? (
			<Link
				to={`/media/${subscription.media_id}`}
				className="relative h-36 w-24 flex-shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-400/15 to-surface-200 shadow-inner transition hover:ring-2 hover:ring-brand-500/60"
				title={`查看「${subscription.name}」媒体详情`}
			>
				{poster}
			</Link>
		) : (
			<div className="relative h-36 w-24 flex-shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-400/15 to-surface-200 shadow-inner">
				{poster}
			</div>
		)}

        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <div className="mb-1 flex flex-wrap gap-1.5">
              <span className="rounded-full bg-primary-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-brand-500">
                {subscription.delivery_mode === 'resource_import' ? '自动追更' : subscription.source || 'RSS'}
              </span>
              <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-semibold text-sand-500">
                {[subscription.media_type, subscription.media_category].filter(Boolean).join(' / ') || '自动分类'}
              </span>
              <span
                className={
                  'rounded-full px-2 py-0.5 text-[10px] font-semibold ' +
                  (subscription.enabled ? 'bg-emerald-50 text-emerald-600' : 'bg-gray-100 text-gray-500')
                }
              >
                {subscription.enabled ? '启用中' : '已停用'}
              </span>
            </div>
            <h2 className="truncate font-display text-lg font-semibold text-ink-600" title={subscription.name}>
              {subscription.name}
            </h2>
            <p className="mt-1 line-clamp-2 text-xs leading-5 text-ink-50">
              {subscription.overview || subscription.filter || '已隐藏订阅源地址，避免多用户场景泄露私有 RSS Token。'}
            </p>
          </div>

          <div className="space-y-1.5 text-xs text-ink-100">
            <div className="flex items-center gap-1.5">
              <ShieldCheck size={13} className="text-brand-500" />
              <span>{subscription.delivery_mode === 'resource_import' ? `${subscription.resource_source === 'pansou' ? 'PanSou' : '常规资源'} · 第 ${subscription.season_number || 1} 季` : '订阅源已脱敏'}</span>
            </div>
            <div className="flex items-center gap-1.5">
              <CalendarClock size={13} className="text-brand-500" />
              <span>
                {subscription.last_run_at ? new Date(subscription.last_run_at).toLocaleString() : '尚未运行'} · {subscription.catch_up_active
                  ? '缺集快速补齐中（每分钟）'
                  : `每 ${subscription.poll_interval_minutes || 180} 分钟`}
              </span>
            </div>
            <div className="flex items-center gap-1.5">
              <CheckCircle2 size={13} className="text-brand-500" />
              <span>{subscriptionProgressLabel(subscription)}</span>
            </div>
			{importJobs.length > 0 && (
              <details className="rounded-xl border border-gray-100 bg-sand-50 px-2.5 py-2">
                <summary className="cursor-pointer text-xs font-semibold text-brand-500">
					查看 {importJobs.length} 条自动入库明细
                </summary>
                <div className="mt-2 space-y-2">
					{visibleImportJobs.map((job) => (
                    <div key={job.id} className="border-t border-sand-200 pt-2 text-xs text-ink-50 first:border-t-0 first:pt-0">
                      <p className="font-medium text-ink-100">第 {job.attempt || 1} 次 · {job.outcome || job.status}</p>
                      {job.candidate_title && <p className="break-all">{job.candidate_title}</p>}
                      {job.selected_episodes?.length ? <p>资源识别：{formatEpisodes(job.selected_episodes)}</p> : null}
                      {job.moved_episodes?.length ? <p>实际补入：{formatEpisodes(job.moved_episodes)}</p> : null}
                      {job.verified_episodes?.length ? <p>最终校验：{formatEpisodes(job.verified_episodes)}</p> : null}
                      {job.scan_added !== undefined ? <p>扫描新增：{job.scan_added} 集</p> : null}
                      {job.error && <p className="break-words text-red-500">{job.error}</p>}
                    </div>
                  ))}
                </div>
				{importPageCount > 1 && (
					<div className="mt-3 flex items-center justify-between gap-2 border-t border-sand-200 pt-2 text-xs text-ink-50">
						<span>第 {visibleImportPage + 1} / {importPageCount} 页</span>
						<div className="flex gap-1.5">
							<button
								type="button"
								className="rounded-lg border border-gray-200 bg-white px-2 py-1 disabled:cursor-not-allowed disabled:opacity-40"
								disabled={visibleImportPage === 0}
								onClick={(event) => {
									event.preventDefault()
									setImportPage((page) => Math.max(0, page - 1))
								}}
							>
								上一页
							</button>
							<button
								type="button"
								className="rounded-lg border border-gray-200 bg-white px-2 py-1 disabled:cursor-not-allowed disabled:opacity-40"
								disabled={visibleImportPage >= importPageCount - 1}
								onClick={(event) => {
									event.preventDefault()
									setImportPage((page) => Math.min(importPageCount - 1, page + 1))
								}}
							>
								下一页
							</button>
						</div>
					</div>
				)}
              </details>
            )}
          </div>

          <div className="flex flex-wrap gap-1.5">
            {subscriptionRuleBadges(subscription).map((label) => (
              <span key={label} className="rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-[10px] text-ink-100">
                {label}
              </span>
            ))}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 border-t border-gray-100 bg-gray-50/70 px-4 py-3">
        <button
          className="rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-ink-100 hover:bg-gray-50"
          onClick={() => onEdit(subscription)}
        >
          <Pencil size={13} className="mr-1 inline" />
          编辑
        </button>
        <button
          className={
            subscription.enabled
              ? 'rounded-xl border border-amber-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-amber-600 hover:bg-amber-400/10'
              : 'rounded-xl border border-emerald-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-emerald-600 hover:bg-emerald-400/10'
          }
          onClick={() => onSetEnabled(subscription, !subscription.enabled)}
        >
          {subscription.enabled ? <Pause size={13} className="mr-1 inline" /> : <Power size={13} className="mr-1 inline" />}
          {subscription.enabled ? '停用' : '启用'}
        </button>
        <button
          className="rounded-xl border border-primary-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-primary-400/10"
          onClick={() => onRunNow(subscription)}
          title="手动立即执行一次检测"
        >
          <Play size={13} className="mr-1 inline" />
          运行
        </button>
        <button
          className="rounded-xl border border-red-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-red-400 hover:bg-red-400/10"
          onClick={() => onRemove(subscription)}
        >
          <Trash2 size={13} className="mr-1 inline" />
          删除
        </button>
      </div>
    </article>
  )
}

function formatEpisodes(values: number[]): string {
  return [...new Set(values.filter((value) => Number.isInteger(value) && value > 0))]
    .sort((left, right) => left - right)
    .map((value) => `E${value}`)
    .join('、')
}
