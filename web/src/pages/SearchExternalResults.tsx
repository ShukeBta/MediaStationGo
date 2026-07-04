import { useState } from 'react'
import { CheckCircle2, Info, Rss } from 'lucide-react'
import toast from 'react-hot-toast'

import type { DiscoverItem } from '../api/discover'
import type { ExternalMediaResult } from '../api/ai'
import { imageURL } from '../api/client'
import { subscriptionsAPI } from '../api/subscriptions'
import { DiscoverSubscriptionRules } from './DiscoverDetailModalSections'
import {
  apiErrorMessage,
  buildDiscoverSubscriptionInput,
  initialDiscoverSubscriptionForm,
} from './discoverDetailModalModel'

export function ExternalResults({
  items,
}: {
  items: ExternalMediaResult[]
}) {
  const [detail, setDetail] = useState<ExternalMediaResult | null>(null)
  return (
    <section className="space-y-3">
      <div>
        <h2 className="font-display text-xl font-semibold text-ink-600">外部数据源</h2>
        <p className="text-xs text-ink-50">
          来自 TMDb / 豆瓣 / Bangumi。电影入队最佳资源；剧集/动漫优先整季或全集包，否则按集批量入队。
        </p>
      </div>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((item) => {
          const keyword = item.subscribe_keyword || item.title
          const key = `${item.source}:${keyword}`
          return (
            <article
              key={key}
              role="button"
              tabIndex={0}
              onClick={() => setDetail(item)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') setDetail(item)
              }}
              className="glass-panel flex cursor-pointer gap-3 !p-3 transition hover:-translate-y-0.5 hover:shadow-lg"
            >
              <div className="h-28 w-20 shrink-0 overflow-hidden rounded-xl bg-gray-100">
                {item.poster_url ? (
                  <img
                    src={imageURL(item.poster_url)}
                    alt={item.title}
                    className="h-full w-full object-cover"
                  />
                ) : null}
              </div>
              <div className="min-w-0 flex-1">
                <div className="mb-1 flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-primary-400/10 px-2 py-0.5 text-[10px] uppercase text-brand-500">
                    {item.source}
                  </span>
                  {item.media_type && <span className="text-xs text-sand-500">{item.media_type}</span>}
                  {item.year ? <span className="text-xs text-sand-500">{item.year}</span> : null}
                  {item.rating ? <span className="text-xs text-amber-500">★ {item.rating.toFixed(1)}</span> : null}
                </div>
                <h3 className="truncate font-semibold text-ink-600">{item.title}</h3>
                <p className="mt-1 line-clamp-2 text-xs text-ink-50">
                  {item.overview || `订阅关键词：${keyword}`}
                </p>
                <div className="mt-2 flex flex-wrap gap-1.5 text-[10px]">
                  <span className={'rounded-full px-2 py-0.5 font-semibold ' + (item.in_library ? 'bg-emerald-50 text-emerald-600' : 'bg-amber-50 text-amber-600')}>
                    {item.in_library ? '本地已入库' : '本地未入库'}
                  </span>
                  {isSeriesItem(item) ? (
                    <span className="rounded-full bg-gray-100 px-2 py-0.5 text-ink-100">
                      已有 {item.downloaded_episodes || 0}/{item.total_episodes || '未知'} 集
                    </span>
                  ) : null}
                </div>
                <button
                  onClick={(event) => {
                    event.stopPropagation()
                    setDetail(item)
                  }}
                  className="mt-3 rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10 disabled:opacity-50"
                >
                  <Rss size={12} className="mr-1 inline" />
                  订阅规则
                </button>
              </div>
            </article>
          )
        })}
      </div>
      {detail && (
        <ExternalDetailModal
          item={detail}
          onClose={() => setDetail(null)}
        />
      )}
    </section>
  )
}

function ExternalDetailModal({
  item,
  onClose,
}: {
  item: ExternalMediaResult
  onClose: () => void
}) {
  const missing = item.missing_episodes ?? []
  const discoverItem = item as unknown as DiscoverItem
  const [form, setForm] = useState(() => initialDiscoverSubscriptionForm(discoverItem))
  const [formBusy, setFormBusy] = useState(false)
  const submit = async () => {
    setFormBusy(true)
    try {
      const sub = await subscriptionsAPI.create(
        buildDiscoverSubscriptionInput(discoverItem, form, item.source || 'tmdb'),
      )
      if (form.run_now) {
        const run = await subscriptionsAPI.runNow(sub.id)
        toast.success(run.queued > 0 ? `已订阅并加入 ${run.queued} 个下载` : '已订阅，暂未命中可下载资源')
      } else {
        toast.success('已创建订阅')
      }
      onClose()
    } catch (err) {
      toast.error(apiErrorMessage(err, '订阅失败'))
    } finally {
      setFormBusy(false)
    }
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm" onClick={onClose}>
      <div className="max-h-[88vh] w-full max-w-3xl overflow-y-auto rounded-3xl bg-white shadow-2xl" onClick={(event) => event.stopPropagation()}>
        <div className="grid gap-0 md:grid-cols-[220px,1fr]">
          <div className="min-h-72 bg-gray-100">
            {item.poster_url ? (
              <img src={imageURL(item.poster_url)} alt={item.title} className="h-full w-full object-cover" />
            ) : (
              <div className="flex h-full min-h-72 items-center justify-center text-brand-500">
                <Info size={42} />
              </div>
            )}
          </div>
          <div className="space-y-4 p-5">
            <div>
              <div className="mb-2 flex flex-wrap gap-2 text-xs">
                <span className="rounded-full bg-primary-400/10 px-2 py-0.5 font-semibold uppercase text-brand-500">{item.source}</span>
                {item.media_type ? <span className="rounded-full bg-gray-100 px-2 py-0.5 text-ink-100">{item.media_type}</span> : null}
                {item.year ? <span className="rounded-full bg-gray-100 px-2 py-0.5 text-ink-100">{item.year}</span> : null}
                {item.rating ? <span className="rounded-full bg-amber-50 px-2 py-0.5 text-amber-600">★ {item.rating.toFixed(1)}</span> : null}
              </div>
              <h3 className="font-display text-2xl font-bold text-ink-600">{item.title}</h3>
              <p className="mt-2 text-sm leading-6 text-ink-50">{item.overview || '暂无简介。'}</p>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              <StatusBox label="入库状态" value={item.in_library ? '已入库' : '未入库'} ok={item.in_library} />
              <StatusBox label="本地条目" value={`${item.local_media_count || 0} 个`} />
              <StatusBox label="剧集进度" value={isSeriesItem(item) ? `${item.downloaded_episodes || 0}/${item.total_episodes || '未知'} 集` : '单部影片'} />
            </div>

            {isSeriesItem(item) && (
              <div className="rounded-2xl border border-gray-100 bg-gray-50 p-3 text-sm">
                <div className="mb-2 font-semibold text-ink-600">缺失情况</div>
                {missing.length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {missing.slice(0, 80).map((episode) => (
                      <span key={episode} className="rounded-full bg-white px-2 py-0.5 text-xs text-ink-100 shadow-sm">第 {episode} 集</span>
                    ))}
                    {missing.length > 80 ? <span className="text-xs text-sand-500">还有 {missing.length - 80} 集…</span> : null}
                  </div>
                ) : item.in_library && item.total_episodes ? (
                  <p className="flex items-center gap-1.5 text-emerald-600"><CheckCircle2 size={15} /> 已完整入库</p>
                ) : (
                  <p className="text-sand-500">总集数未知，订阅时会跳过本地已有单集，优先补新集。</p>
                )}
              </div>
            )}

            <DiscoverSubscriptionRules
              form={form}
              busy={formBusy}
              onChange={(patch) => setForm((current) => ({ ...current, ...patch }))}
              onSubmit={submit}
            />

            <div className="flex justify-end pt-2">
              <button onClick={onClose} className="rounded-xl border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50">关闭</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function StatusBox({ label, value, ok }: { label: string; value: string; ok?: boolean }) {
  return (
    <div className="rounded-2xl border border-gray-100 bg-gray-50 p-3">
      <div className="text-xs text-sand-500">{label}</div>
      <div className={'mt-1 font-semibold ' + (ok ? 'text-emerald-600' : 'text-ink-600')}>{value}</div>
    </div>
  )
}

function isSeriesItem(item: ExternalMediaResult) {
  return ['tv', 'anime', 'variety'].includes((item.media_type || '').toLowerCase())
}
