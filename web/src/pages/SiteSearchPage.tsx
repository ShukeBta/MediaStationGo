import { FormEvent, useState } from 'react'
import toast from 'react-hot-toast'
import { Download, Search } from 'lucide-react'

import { sitesAPI, type SiteSearchResult } from '../api/sites'
import { downloadsAPI } from '../api/downloads'

function fmtBytes(n: number): string {
  if (!n || n <= 0) return '—'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${u[i]}`
}

// SiteSearchPage performs cross-site torrent search across all enabled
// sites and displays a merged result table sorted by seeders.
export function SiteSearchPage() {
  const [keyword, setKeyword] = useState('')
  const [results, setResults] = useState<SiteSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!keyword.trim()) return
    setLoading(true)
    setSearched(true)
    try {
      const data = await sitesAPI.search(keyword.trim())
      setResults(data.items || [])
      if ((data.items || []).length === 0) {
        toast('未找到结果,请检查站点配置或更换关键词')
      }
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '搜索失败'
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }

  const downloadTorrent = async (item: SiteSearchResult) => {
    const url = item.download_url || item.torrent_url
    if (!url) {
      toast.error('无可用下载链接')
      return
    }
    try {
      await downloadsAPI.add(url, '', {
        title: item.title,
        source_category: item.category,
      })
      toast.success(`已加入下载: ${item.title.substring(0, 40)}...`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '下载失败'
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex items-center gap-3">
        <Search className="h-6 w-6 text-brand-500" />
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">站点搜索</h1>
          <p className="text-sm text-ink-50">
            跨站搜索所有已启用的 PT/BT 站点,结果按做种数排序。
          </p>
        </div>
      </header>

      <form onSubmit={onSubmit} className="flex flex-wrap gap-2">
        <input
          autoFocus
          className="input-base flex-1"
          placeholder="输入关键词搜索(如: 流浪地球 / Dune 2024)"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <button type="submit" disabled={loading} className="neon-button">
          {loading ? '搜索中…' : '搜索'}
        </button>
      </form>

      {loading && <p className="text-sand-500">正在查询所有站点…</p>}

      {!loading && searched && results.length === 0 && (
        <div className="glass-panel">
          <p className="text-ink-100">
            未找到结果。请确认已在「站点管理」中添加并启用了至少一个站点,且 Cookie / API Key 有效。
          </p>
        </div>
      )}

      {results.length > 0 && (
        <div className="glass-panel overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="py-2">站点</th>
                <th>标题</th>
                <th>大小</th>
                <th>S</th>
                <th>L</th>
                <th>Free</th>
                <th className="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {results.map((item, idx) => (
                <tr key={idx} className="border-t border-gray-200 align-top">
                  <td className="py-2 text-brand-500">{item.site_name}</td>
                  <td className="max-w-md py-2">
                    <a
                      href={item.torrent_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="font-semibold text-ink-600 transition hover:text-brand-500"
                      title={item.title}
                    >
                      {item.title.length > 80
                        ? item.title.substring(0, 80) + '…'
                        : item.title}
                    </a>
                    {item.subtitle && (
                      <div className="mt-1 line-clamp-2 text-xs text-ink-50" title={item.subtitle}>
                        {item.subtitle}
                      </div>
                    )}
                  </td>
                  <td className="whitespace-nowrap text-ink-100">
                    {fmtBytes(item.size)}
                  </td>
                  <td className="text-emerald-400">{item.seeders || '—'}</td>
                  <td className="text-red-400">{item.leechers || '—'}</td>
                  <td>
                    {item.free && (
                      <span className="rounded-lg border border-emerald-400/40 px-1.5 py-0.5 text-xs text-emerald-400">
                        Free
                      </span>
                    )}
                  </td>
                  <td className="py-2 text-right">
                    <button
                      onClick={() => downloadTorrent(item)}
                      className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                      title="加入下载"
                    >
                      <Download size={12} className="inline" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
