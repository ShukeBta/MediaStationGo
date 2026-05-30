import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { CalendarClock, CheckCircle2, Film, Pencil, Play, Plus, Save, ShieldCheck, Trash2 } from 'lucide-react'

import { subscriptionsAPI } from '../api/subscriptions'
import { imageURL } from '../api/client'
import { confirmAction } from '../components/ConfirmDialog'
import type { Subscription } from '../types'

export function SubscriptionsPage() {
  const [items, setItems] = useState<Subscription[]>([])
  const [name, setName] = useState('')
  const [feed, setFeed] = useState('')
  const [filter, setFilter] = useState('')
  const [mediaType, setMediaType] = useState('')
  const [mediaCategory, setMediaCategory] = useState('')
  const [savePath, setSavePath] = useState('')
  const [searchMode, setSearchMode] = useState('keyword')
  const [imdbID, setImdbID] = useState('')
  const [resolution, setResolution] = useState('best')
  const [quality, setQuality] = useState('')
  const [effects, setEffects] = useState('')
  const [releaseGroups, setReleaseGroups] = useState('')
  const [excludeWords, setExcludeWords] = useState('cam,ts,tc,枪版')
  const [washEnabled, setWashEnabled] = useState(false)
  const [washPriority, setWashPriority] = useState('balanced')
  const [editingId, setEditingId] = useState('')
  const [loading, setLoading] = useState(true)

  const refresh = () =>
    subscriptionsAPI
      .list()
      .then(setItems)
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const payload = {
        name,
        feed_url: feed,
        filter,
        media_type: mediaType || undefined,
        media_category: mediaCategory || undefined,
        save_path: savePath || undefined,
        search_mode: searchMode,
        imdb_id: imdbID || undefined,
        resolution,
        // 规则类字段发送原始字符串（含空串），以便编辑时可清空/修改；create 存空串无害，update 才能真正生效。
        quality,
        effects,
        release_groups: releaseGroups,
        exclude_words: excludeWords,
        wash_enabled: washEnabled,
        wash_priority: washPriority,
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
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '创建失败'
      toast.error(msg)
    }
  }

  const resetForm = () => {
    setEditingId('')
    setName('')
    setFeed('')
    setFilter('')
    setMediaType('')
    setMediaCategory('')
    setSavePath('')
    setSearchMode('keyword')
    setImdbID('')
    setResolution('best')
    setQuality('')
    setEffects('')
    setReleaseGroups('')
    setExcludeWords('cam,ts,tc,枪版')
    setWashEnabled(false)
    setWashPriority('balanced')
  }

  const startEdit = (s: Subscription) => {
    setEditingId(s.id)
    setName(s.name)
    setFeed(s.feed_url)
    setFilter(s.filter || '')
    setMediaType(s.media_type || '')
    setMediaCategory(s.media_category || '')
    setSavePath(s.save_path || '')
    setSearchMode(s.search_mode || 'keyword')
    setImdbID(s.imdb_id || '')
    setResolution(s.resolution || 'best')
    setQuality(s.quality || '')
    setEffects(s.effects || '')
    setReleaseGroups(s.release_groups || '')
    setExcludeWords(s.exclude_words || '')
    setWashEnabled(Boolean(s.wash_enabled))
    setWashPriority(s.wash_priority || 'balanced')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">RSS 订阅</h1>
      <p className="text-sm text-ink-50">
        定期轮询 RSS 源(每 10 分钟一次),将匹配过滤器的项目自动加入下载队列；启用智能分类后会按二级分类写入下载目录。
      </p>

      <form onSubmit={onCreate} className="glass-panel grid gap-3 md:grid-cols-4">
        <input
          required
          className="input-base"
          placeholder="名称(显示用)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          required
          className="input-base"
          placeholder="RSS 地址"
          value={feed}
          onChange={(e) => setFeed(e.target.value)}
        />
        <input
          className="input-base"
          placeholder="过滤器(正则,可选)"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <select className="input-base" value={mediaType} onChange={(e) => setMediaType(e.target.value)}>
          <option value="">自动识别类型</option>
          <option value="movie">电影</option>
          <option value="tv">电视剧</option>
          <option value="anime">动漫</option>
          <option value="variety">综艺</option>
        </select>
        <input
          className="input-base"
          placeholder="二级分类覆盖(如 综艺/日番,可选)"
          value={mediaCategory}
          onChange={(e) => setMediaCategory(e.target.value)}
        />
        <input
          className="input-base md:col-span-2"
          placeholder="下载根目录覆盖(可选,默认使用下载器保存路径)"
          value={savePath}
          onChange={(e) => setSavePath(e.target.value)}
        />
        <select className="input-base" value={searchMode} onChange={(e) => setSearchMode(e.target.value)}>
          <option value="keyword">标题关键词搜索</option>
          <option value="imdb">IMDB ID 搜索</option>
        </select>
        <input className="input-base" placeholder="IMDB ID，如 tt1160419" value={imdbID} onChange={(e) => setImdbID(e.target.value)} />
        <select className="input-base" value={resolution} onChange={(e) => setResolution(e.target.value)}>
          <option value="best">分辨率自动择优</option>
          <option value="2160p">2160p / 4K</option>
          <option value="1080p">1080p</option>
          <option value="720p">720p</option>
        </select>
        <select className="input-base" value={quality} onChange={(e) => setQuality(e.target.value)}>
          <option value="">质量不限</option>
          <option value="remux">REMUX</option>
          <option value="bluray">BluRay</option>
          <option value="web-dl">WEB-DL</option>
          <option value="hdtv">HDTV</option>
        </select>
        <input className="input-base" placeholder="特效/音轨 hdr,dolby-vision,atmos" value={effects} onChange={(e) => setEffects(e.target.value)} />
        <label className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
          <input type="checkbox" checked={washEnabled} onChange={(e) => setWashEnabled(e.target.checked)} />
          启用洗版择优
        </label>
        <select className="input-base disabled:opacity-50" disabled={!washEnabled} value={washPriority} onChange={(e) => setWashPriority(e.target.value)}>
          <option value="balanced">洗版：均衡</option>
          <option value="resolution">洗版：分辨率优先</option>
          <option value="quality">洗版：片源质量优先</option>
          <option value="effects">洗版：HDR/DV/Atmos 优先</option>
          <option value="seeders">洗版：做种数优先</option>
        </select>
        <input className="input-base" placeholder="发布组白名单，如 FRDS,OurTV" value={releaseGroups} onChange={(e) => setReleaseGroups(e.target.value)} />
        <input className="input-base" placeholder="排除词，如 cam,ts,tc" value={excludeWords} onChange={(e) => setExcludeWords(e.target.value)} />
        <button type="submit" className="neon-button md:col-span-1">
          {editingId ? <Save size={16} /> : <Plus size={16} />}
          {editingId ? '保存' : '添加'}
        </button>
        {editingId && (
          <button type="button" onClick={resetForm} className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:bg-gray-50">
            取消编辑
          </button>
        )}
      </form>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && items.length === 0 && <p className="text-ink-50">暂无订阅。</p>}

      {items.length > 0 && (
        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((subscription) => (
            <article
              key={subscription.id}
              className="group overflow-hidden rounded-3xl border border-white/70 bg-white shadow-sm transition hover:-translate-y-1 hover:shadow-xl"
            >
              <div className="relative flex gap-4 p-4">
                <div className="relative h-36 w-24 flex-shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-400/15 to-surface-200 shadow-inner">
                  {subscription.poster_url ? (
                    <img
                      src={imageURL(subscription.poster_url)}
                      alt={subscription.name}
                      className="h-full w-full object-cover"
                    />
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
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-semibold ' + (subscription.enabled ? 'bg-emerald-50 text-emerald-600' : 'bg-gray-100 text-gray-500')}>
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
                  onClick={() => startEdit(subscription)}
                >
                  <Pencil size={13} className="mr-1 inline" />
                  编辑
                </button>
                <button
                  className="rounded-xl border border-primary-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-primary-400/10"
                  onClick={async () => {
                    const result = await subscriptionsAPI.runNow(subscription.id)
                    toast.success(`已加入 ${result.queued} 项`)
                  }}
                >
                  <Play size={13} className="mr-1 inline" />
                  运行
                </button>
                <button
                  className="rounded-xl border border-red-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-red-400 hover:bg-red-400/10"
                  onClick={async () => {
                    if (!(await confirmAction({ title: '删除订阅', message: `删除订阅「${subscription.name}」?`, confirmText: '删除' }))) return
                    await subscriptionsAPI.remove(subscription.id)
                    toast.success('已删除')
                    await refresh()
                  }}
                >
                  <Trash2 size={13} className="mr-1 inline" />
                  删除
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}

function subscriptionRuleBadges(subscription: Subscription): string[] {
  const labels = [
    subscription.search_mode === 'imdb' ? `IMDB ${subscription.imdb_id || '未填'}` : '关键词搜索',
    subscription.resolution && subscription.resolution !== 'best' ? subscription.resolution : '分辨率不限',
    subscription.quality || '质量不限',
    subscription.effects || '',
    subscription.release_groups ? `发布组 ${subscription.release_groups}` : '',
    subscription.wash_enabled ? `洗版 ${washPriorityLabel(subscription.wash_priority)}` : '未启用洗版',
  ]
  return labels.filter(Boolean)
}

function subscriptionProgressLabel(subscription: Subscription): string {
  const isSeries = ['tv', 'anime', 'variety'].includes((subscription.media_type || '').toLowerCase())
  if (!isSeries) {
    return subscription.in_library ? '本地已入库' : '本地未入库'
  }
  const downloaded = subscription.downloaded_episodes || 0
  const total = subscription.total_episodes || 0
  if (total > 0) {
    const missing = subscription.missing_episodes?.length || 0
    return missing > 0 ? `已下载 ${downloaded}/${total} 集，缺 ${missing} 集` : `已下载 ${downloaded}/${total} 集`
  }
  return `已下载 ${downloaded}/未知 集`
}

function washPriorityLabel(priority?: string): string {
  switch (priority) {
    case 'resolution':
      return '分辨率优先'
    case 'quality':
      return '片源质量优先'
    case 'effects':
      return '特效优先'
    case 'seeders':
      return '做种数优先'
    default:
      return '均衡'
  }
}
