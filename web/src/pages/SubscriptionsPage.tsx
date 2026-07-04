import { FormEvent, useEffect, useState } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { subscriptionsAPI } from '../api/subscriptions'
import { confirmAction } from '../components/confirmAction'
import type { Subscription } from '../types'
import { SubscriptionCard } from './SubscriptionCard'
import { SubscriptionForm } from './SubscriptionForm'
import { SubscriptionHistorySection } from './SubscriptionHistorySection'
import { defaultSubscriptionFormValues, type SubscriptionFormValues } from './subscriptionFormModel'

export function SubscriptionsPage() {
  const [items, setItems] = useState<Subscription[]>([])
  const [historyItems, setHistoryItems] = useState<Subscription[]>([])
  const [formValues, setFormValues] = useState<SubscriptionFormValues>(defaultSubscriptionFormValues)
  const [editingId, setEditingId] = useState('')
  const [loading, setLoading] = useState(true)
  const [historyLoading, setHistoryLoading] = useState(true)
  const [listError, setListError] = useState('')
  const [historyError, setHistoryError] = useState('')

  const refresh = async () => {
    setLoading(true)
    setHistoryLoading(true)
    setListError('')
    setHistoryError('')

    const historyPromise = subscriptionsAPI
      .history()
      .then((history) => {
        setHistoryItems(history)
      })
      .catch((err: unknown) => {
        const msg = apiErrorMessage(err, '订阅历史加载失败')
        setHistoryError(msg)
        toast.error(msg)
      })
      .finally(() => setHistoryLoading(false))

    try {
      const active = await subscriptionsAPI.list()
      setItems(active)
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '订阅列表加载失败')
      setListError(msg)
      setItems([])
      toast.error(msg)
    } finally {
      setLoading(false)
    }
    await historyPromise
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const payload = {
        name: formValues.name,
        feed_url: formValues.feed,
        filter: formValues.filter,
        media_type: formValues.mediaType || undefined,
        media_category: formValues.mediaCategory || undefined,
        save_path: formValues.savePath || undefined,
        search_mode: formValues.searchMode,
        imdb_id: formValues.imdbID || undefined,
        resolution: formValues.resolution,
        // 规则类字段发送原始字符串（含空串），以便编辑时可清空/修改；create 存空串无害，update 才能真正生效。
        quality: formValues.quality,
        effects: formValues.effects,
        release_groups: formValues.releaseGroups,
        exclude_words: formValues.excludeWords,
        min_seeders: numericRuleValue(formValues.minSeeders),
        max_seeders: numericRuleValue(formValues.maxSeeders),
        min_size_gb: numericRuleValue(formValues.minSizeGB),
        max_size_gb: numericRuleValue(formValues.maxSizeGB),
        free_only: formValues.freeOnly,
        wash_enabled: formValues.washEnabled,
        wash_priority: formValues.washPriority,
        priority: 50,
      }
      if (editingId) {
        await subscriptionsAPI.update(editingId, payload)
        toast.success('已更新订阅')
      } else {
        await subscriptionsAPI.create(payload)
        toast.success('已创建订阅')
      }
      resetForm()
      await refresh()
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '创建失败')
      toast.error(msg)
    }
  }

  const resetForm = () => {
    setEditingId('')
    setFormValues(defaultSubscriptionFormValues)
  }

  const startEdit = (s: Subscription) => {
    setEditingId(s.id)
    setFormValues({
      name: s.name,
      feed: s.feed_url,
      filter: s.filter || '',
      mediaType: s.media_type || '',
      mediaCategory: s.media_category || '',
      savePath: s.save_path || '',
      searchMode: s.search_mode || 'keyword',
      imdbID: s.imdb_id || '',
      resolution: s.resolution || 'best',
      quality: s.quality || '',
      effects: s.effects || '',
      releaseGroups: s.release_groups || '',
      excludeWords: s.exclude_words || '',
      minSeeders: stringRuleValue(s.min_seeders),
      maxSeeders: stringRuleValue(s.max_seeders),
      minSizeGB: stringRuleValue(s.min_size_gb),
      maxSizeGB: stringRuleValue(s.max_size_gb),
      freeOnly: Boolean(s.free_only),
      washEnabled: Boolean(s.wash_enabled),
      washPriority: s.wash_priority || 'balanced',
    })
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const updateFormValue = <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => {
    setFormValues((current) => ({ ...current, [key]: value }))
  }

  const restoreHistorySubscription = async (subscription: Subscription, runAfterRestore = false) => {
    try {
      const restored = await subscriptionsAPI.restore(subscription.id)
      if (runAfterRestore) {
        const result = await subscriptionsAPI.runNow(restored.id)
        toast.success(`已恢复订阅并加入 ${result.queued} 项`)
      } else {
        toast.success('已恢复到正在订阅')
      }
      await refresh()
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '恢复订阅失败')
      toast.error(msg)
    }
  }

  const runSubscriptionNow = async (subscription: Subscription) => {
    try {
      const result = await subscriptionsAPI.runNow(subscription.id)
      toast.success(`已加入 ${result.queued} 项`)
      await refresh()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '运行订阅失败'))
    }
  }

  const removeSubscription = async (subscription: Subscription) => {
    if (!(await confirmAction({ title: '删除订阅', message: `删除订阅「${subscription.name}」?`, confirmText: '删除' }))) return
    try {
      await subscriptionsAPI.remove(subscription.id)
      toast.success('已删除')
      await refresh()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '删除失败'))
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">RSS 订阅</h1>
      <p className="text-sm text-ink-50">
        后台统一每 3 小时轮询 RSS / 站点搜索订阅；手动立即执行不受影响。匹配过滤器的项目会自动加入下载队列，启用智能分类后按二级分类写入下载目录。
      </p>

      <SubscriptionForm
        values={formValues}
        editing={Boolean(editingId)}
        onSubmit={onCreate}
        onCancelEdit={resetForm}
        onChange={updateFormValue}
      />

      <div className="flex items-center justify-between gap-3">
        <h2 className="font-display text-xl font-semibold text-ink-600">正在订阅</h2>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-xl border border-primary-400/40 bg-white px-3 py-2 text-xs font-semibold text-brand-500 hover:bg-primary-400/10 disabled:cursor-not-allowed disabled:opacity-50"
          onClick={() => refresh().catch(() => undefined)}
          disabled={loading || historyLoading}
        >
          <RefreshCw size={14} className={loading || historyLoading ? 'animate-spin' : ''} />
          刷新
        </button>
      </div>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && listError && <SubscriptionLoadError message={listError} onRetry={refresh} />}
      {!loading && !listError && items.length === 0 && <p className="text-ink-50">暂无订阅。</p>}

      {!listError && items.length > 0 && (
        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((subscription) => (
            <SubscriptionCard
              key={subscription.id}
              subscription={subscription}
              onEdit={startEdit}
              onRunNow={runSubscriptionNow}
              onRemove={removeSubscription}
            />
          ))}
        </div>
      )}

      <SubscriptionHistorySection
        subscriptions={historyItems}
        loading={historyLoading}
        error={historyError}
        onRefresh={refresh}
        onRestore={restoreHistorySubscription}
      />
    </div>
  )
}

function SubscriptionLoadError({ message, onRetry }: { message: string; onRetry: () => Promise<void> }) {
  return (
    <div className="rounded-2xl border border-red-300/70 bg-red-50 px-4 py-3 text-sm text-red-700">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <AlertTriangle size={16} className="shrink-0" />
          <span className="break-words">订阅列表加载失败：{message}</span>
        </div>
        <button
          type="button"
          className="rounded-xl border border-red-300 bg-white px-3 py-1.5 text-xs font-semibold text-red-600 hover:bg-red-100"
          onClick={() => onRetry().catch(() => undefined)}
        >
          重试
        </button>
      </div>
    </div>
  )
}

function apiErrorMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: { error?: string; message?: string }; status?: number } })?.response
  if (data?.data?.error) return data.data.error
  if (data?.data?.message) return data.data.message
  if (data?.status) return `${fallback} (${data.status})`
  if ((err as { code?: string })?.code === 'ECONNABORTED') return '请求超时，请检查服务或网络'
  return fallback
}

function numericRuleValue(value: string): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function stringRuleValue(value?: number): string {
  return value && value > 0 ? String(value) : ''
}
