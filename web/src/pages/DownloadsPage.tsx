import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Activity, Download, Search } from 'lucide-react'

import { downloadsAPI } from '../api/downloads'
import { useAuthStore } from '../stores/auth'
import { usePermission } from '../hooks/usePermission'
import { confirmAction } from '../components/confirmAction'
import type { DownloadTask, QBitTorrent } from '../types'
import { DownloadTaskCard } from './DownloadTaskCard'
import { CloudImportDownloadsSection } from './CloudImportDownloadsSection'
import { toLiveCard, toTaskCard } from './downloadTaskCardModel'

export function DownloadsPage() {
  const role = useAuthStore((s) => s.user?.role)
  const isAdmin = role === 'admin'
  const canSearchSites = usePermission('can_manage_sites')
  const [tasks, setTasks] = useState<DownloadTask[]>([])
  const [torrents, setTorrents] = useState<QBitTorrent[] | null>(null)
  const [url, setURL] = useState('')
  const [savePath, setSavePath] = useState('')
  const [adding, setAdding] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      const d = await downloadsAPI.list()
      setTasks(d.tasks)
      setTorrents(d.torrents)
      setError('')
    } catch (requestError) {
      setError((requestError as { response?: { data?: { error?: string } } })?.response?.data?.error || '下载任务加载失败，请检查下载器连接后重试。')
    } finally {
      setLoaded(true)
    }
  }, [])

  useEffect(() => {
    if (!isAdmin) return
    void refresh()
    const id = window.setInterval(() => void refresh(), 5_000)
    return () => window.clearInterval(id)
  }, [isAdmin, refresh])

  const onAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (adding || !url.trim()) return
    setAdding(true)
    try {
      await downloadsAPI.add(url.trim(), savePath.trim())
      toast.success('已加入下载队列')
      setURL('')
      setSavePath('')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '提交失败'
      toast.error(msg)
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Activity className="h-6 w-6 text-brand-500" />
          <h1 className="font-display text-3xl font-bold text-ink-600">下载中心</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canSearchSites && <Link to="/site-search" className="btn-primary px-4 py-2.5"><Search size={17} />搜索 PT / 订阅</Link>}
          {isAdmin && <Link to="/subscriptions" className="btn-outline px-4 py-2.5">管理自动追更</Link>}
        </div>
      </header>

      <p className="text-sm text-sand-500">在 PT 站点搜索资源并下载，或创建订阅持续追更。下载任务在下方自动更新。</p>

      {isAdmin && (
        <div className="space-y-6">

            <form onSubmit={onAdd} className="glass-panel grid gap-3 md:grid-cols-[1fr_1fr_auto]">
              <h2 className="font-display text-lg font-semibold text-ink-600 md:col-span-3">添加下载链接</h2>
              <input
                required
                aria-label="下载链接"
                disabled={adding}
                className="input-base md:col-span-2"
                placeholder="磁力链接 / .torrent URL（提交后不会在页面公开显示）"
                value={url}
                onChange={(e) => setURL(e.target.value)}
              />
              <input
                aria-label="保存路径"
                disabled={adding}
                className="input-base"
                placeholder="保存路径 (可选)"
                value={savePath}
                onChange={(e) => setSavePath(e.target.value)}
              />
              <button type="submit" className="neon-button md:col-span-3" disabled={adding || !url.trim()}>
                <Download size={16} /> {adding ? '正在添加…' : '添加下载'}
              </button>
            </form>

            <section className="space-y-3">
              {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
              <div className="flex items-center justify-between">
                <h2 className="font-display text-xl font-semibold text-ink-600">当前下载</h2>
                <span className="text-xs text-ink-50">每 5 秒自动刷新</span>
              </div>
              {!loaded && <p className="text-sm text-sand-500">正在加载下载任务…</p>}
              {loaded && !error && torrents === null && (
                <div className="glass-panel text-sand-500">
                  尚未连接到下载器，请到{' '}
                  <Link to="/download-clients" className="text-brand-500 hover:underline">
                    下载器
                  </Link>{' '}
                  页面添加并测试连接。
                </div>
              )}
              {loaded && !error && torrents && torrents.length === 0 && (
                <div className="glass-panel text-sand-500">暂无运行中任务。</div>
              )}
              {torrents && torrents.length > 0 && (
                <div className="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">
                  {torrents.map((torrent) => (
                    <DownloadTaskCard
                      key={`${torrent.client_id}:${torrent.hash}`}
                      item={toLiveCard(torrent)}
                      removable
                      onRemove={async () => {
                        if (!(await confirmAction({ title: '删除下载任务', message: `删除「${torrent.title || torrent.name}」?`, confirmText: '删除' }))) return
                        await downloadsAPI.remove(torrent.hash, torrent.client_id, false)
                        toast.success('已删除任务')
                        await refresh()
                      }}
                    />
                  ))}
                </div>
              )}
            </section>

            <section className="space-y-3">
              <h2 className="font-display text-xl font-semibold text-ink-600">下载历史</h2>
              {loaded && !error && tasks.length === 0 ? (
                <div className="glass-panel text-sand-500">暂无历史下载。</div>
              ) : (
                <div className="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">
                  {tasks.map((task) => (
                    <DownloadTaskCard key={task.id} item={toTaskCard(task)} />
                  ))}
                </div>
              )}
            </section>
        </div>
      )}

      {!isAdmin && <p className="text-sm text-sand-500">下载器任务由管理员管理；有 PT 站点权限的账号可以通过上方入口搜索和提交资源。</p>}
      <CloudImportDownloadsSection isAdmin={isAdmin} />
    </div>
  )
}
