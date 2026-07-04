import { CalendarClock, CheckCircle2, Film, Pencil, Play, ShieldCheck, Trash2 } from 'lucide-react'

import { imageURL } from '../api/client'
import type { Subscription } from '../types'
import { subscriptionProgressLabel, subscriptionRuleBadges } from './subscriptionPageModel'

interface SubscriptionCardProps {
  subscription: Subscription
  onEdit: (subscription: Subscription) => void
  onRunNow: (subscription: Subscription) => void
  onRemove: (subscription: Subscription) => void
}

export function SubscriptionCard({ subscription, onEdit, onRunNow, onRemove }: SubscriptionCardProps) {
  return (
    <article className="group overflow-hidden rounded-3xl border border-white/70 bg-white shadow-sm transition hover:-translate-y-1 hover:shadow-xl">
      <div className="relative flex gap-4 p-4">
        <div className="relative h-36 w-24 flex-shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-400/15 to-surface-200 shadow-inner">
          {subscription.poster_url ? (
            <img src={imageURL(subscription.poster_url, subscription.updated_at)} alt={subscription.name} className="h-full w-full object-cover" />
          ) : (
            <div className="flex h-full w-full flex-col items-center justify-center gap-2 px-2 text-center text-xs font-semibold text-brand-500">
              <Film size={22} />
              <span className="line-clamp-3">{subscription.name}</span>
            </div>
          )}
        </div>

        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <div className="mb-1 flex flex-wrap gap-1.5">
              <span className="rounded-full bg-primary-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-brand-500">
                {subscription.source || 'RSS'}
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
              <span>订阅源已脱敏</span>
            </div>
            <div className="flex items-center gap-1.5">
              <CalendarClock size={13} className="text-brand-500" />
              <span>{subscription.last_run_at ? new Date(subscription.last_run_at).toLocaleString() : '尚未运行'}</span>
            </div>
            <div className="flex items-center gap-1.5">
              <CheckCircle2 size={13} className="text-brand-500" />
              <span>{subscriptionProgressLabel(subscription)}</span>
            </div>
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
          className="rounded-xl border border-primary-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-primary-400/10"
          onClick={() => onRunNow(subscription)}
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
