import { FormEvent, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { BellPlus, Download, LoaderCircle, Search } from 'lucide-react'
import toast from 'react-hot-toast'

import { sitesAPI, type SiteSearchResult } from '../api/sites'
import { usePermission } from '../hooks/usePermission'
import { useAuthStore } from '../stores/auth'
import { LocalDownloadPathField } from './LocalDownloadPathField'
import { ptDownloadInput, ptMetadataForQuery, ptResourceSize, ptSubscriptionInput, ptSubscriptionResultMessage, type PTResourceMetadata } from './ptResourceModel'

export function PTResourceSearchPanel({ initialQuery = '', metadata = {} }: {
  initialQuery?: string
  metadata?: PTResourceMetadata
}) {
  const canSearch = usePermission('can_manage_sites')
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [query, setQuery] = useState(initialQuery)
  const [items, setItems] = useState<SiteSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [error, setError] = useState('')
  const [acting, setActing] = useState('')
  const [subscribed, setSubscribed] = useState(false)
  const [season, setSeason] = useState(String(metadata.season_number || 1))
  // Catalogue totals may cover every season. Only an explicitly entered
  // season total may stop this subscription when its episodes are complete.
  const [total, setTotal] = useState('')
  const [savePath, setSavePath] = useState('')
  const sequence = useRef(0)
  const activeMetadata = ptMetadataForQuery(query, initialQuery, metadata)
  const series = ['tv', 'anime', 'variety'].includes(activeMetadata.media_type || '')

  const search = async (event: FormEvent) => {
    event.preventDefault()
    if (!query.trim() || loading) return
    const request = ++sequence.current
    setLoading(true)
    setSearched(true)
    setError('')
    setItems([])
    try {
      const result = await sitesAPI.search(query.trim())
      if (sequence.current !== request) return
      setItems(result.items ?? [])
      if (result.error) setError(result.error)
    } catch (requestError) {
      if (sequence.current === request) setError(ptRequestError(requestError, '站点搜索失败'))
    } finally {
      if (sequence.current === request) setLoading(false)
    }
  }

  const download = async (item: SiteSearchResult, key: string) => {
    if (acting) return
    setActing(key)
    try {
      await sitesAPI.download(ptDownloadInput(item, activeMetadata, savePath))
      toast.success('已加入下载中心')
    } catch (requestError) {
      toast.error(ptRequestError(requestError, '加入下载失败'))
    } finally {
      setActing('')
    }
  }

  const subscribe = async () => {
    if (acting || !query.trim() || subscribed) return
    setActing('subscribe')
    try {
      const result = await sitesAPI.subscribe(ptSubscriptionInput(query, activeMetadata, season, total, savePath))
      setSubscribed(true)
      const message = ptSubscriptionResultMessage(result)
      if (result.run_error) toast.error(message)
      else toast.success(message)
    } catch (requestError) {
      toast.error(ptRequestError(requestError, '创建订阅失败'))
    } finally {
      setActing('')
    }
  }

  if (!canSearch) return <p className="text-sm text-sand-500">当前账号未开放 PT 站点搜索权限，请联系管理员。</p>

  return (
    <section className="space-y-3">
      <p className="text-xs text-sand-500">搜索已配置的 PT 站点，选择资源下载，或按作品创建持续订阅。下载使用已配置的下载器。</p>
      <form className="flex gap-2" onSubmit={search}>
        <input aria-label="PT 搜索关键词" className="input-base min-w-0 flex-1" value={query} disabled={Boolean(acting) || loading} placeholder="作品名称或资源关键词" onChange={(event) => { setQuery(event.target.value); setSubscribed(false); setItems([]); setSearched(false); setError('') }} />
        <button type="submit" className="btn-primary shrink-0 gap-2" disabled={loading || !query.trim()}>
          {loading ? <LoaderCircle size={16} className="animate-spin" /> : <Search size={16} />}
          搜索 PT
        </button>
      </form>
      <LocalDownloadPathField value={savePath} onChange={setSavePath} disabled={Boolean(acting) || subscribed} canManage={isAdmin} />
      <p className="text-xs text-sand-500">订阅使用此目录作为根目录，启用智能分类时追加分类子目录。</p>
      {query.trim() && <p className="text-xs text-sand-500">订阅名称：{activeMetadata.title?.trim() || query.trim()}</p>}
      <div className="flex flex-wrap items-end gap-2">
          {series && <>
            <label className="text-xs text-sand-500">季数<input className="input-base mt-1 w-24" type="number" min={1} disabled={Boolean(acting)} value={season} onChange={(event) => { setSeason(event.target.value); setTotal(''); setSubscribed(false) }} /></label>
            <label className="text-xs text-sand-500">本季总集数<input className="input-base mt-1 w-28" type="number" min={0} disabled={Boolean(acting)} placeholder="未知可留空" value={total} onChange={(event) => { setTotal(event.target.value); setSubscribed(false) }} /></label>
          </>}
          <button type="button" className="btn-outline gap-2" disabled={Boolean(acting) || !query.trim() || subscribed} onClick={() => void subscribe()}>
            {acting === 'subscribe' ? <LoaderCircle size={15} className="animate-spin" /> : <BellPlus size={15} />}
            {subscribed ? '已创建订阅' : series ? 'PT 自动追更' : '订阅此关键词'}
          </button>
          {isAdmin && <Link to="/subscriptions" className="py-2 text-xs text-brand-500">管理订阅规则</Link>}
      </div>
      {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
      {!loading && searched && !error && items.length === 0 && <p className="text-sm text-sand-500">未找到资源，可更换关键词，或检查 PT 站点是否启用及登录凭据是否有效。</p>}
      {items.length > 0 && <div className="max-h-[55vh] space-y-2 overflow-y-auto">
        {items.map((item, index) => {
          const key = `${item.site_id}:${item.id || index}`
          return <article key={key} className="rounded-xl border border-gray-200 p-3">
            <h4 className="break-words text-sm font-semibold text-ink-600">{item.title}</h4>
            {item.subtitle && <p className="mt-1 line-clamp-2 text-xs text-sand-500">{item.subtitle}</p>}
            <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs text-sand-500">{item.site_name} · {ptResourceSize(item.size)} · {item.seeders || 0} 做种{item.free ? ' · 免费' : ''}</span>
              <button type="button" className="btn-outline gap-1 px-3 py-1.5 text-xs" disabled={Boolean(acting)} onClick={() => void download(item, key)}>
                {acting === key ? <LoaderCircle size={13} className="animate-spin" /> : <Download size={13} />}下载
              </button>
            </div>
          </article>
        })}
      </div>}
    </section>
  )
}

function ptRequestError(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback
}
