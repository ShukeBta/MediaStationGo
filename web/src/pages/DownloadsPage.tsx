import { FormEvent, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Download, ShieldCheck } from 'lucide-react'

import { downloadsAPI } from '../api/downloads'
import { useAuthStore } from '../stores/auth'
import { confirmAction } from '../components/confirmAction'
import type { DownloadTask, QBitTorrent } from '../types'
import { DownloadTaskCard } from './DownloadTaskCard'
import { toLiveCard, toTaskCard } from './downloadTaskCardModel'

export function DownloadsPage() {
  const role = useAuthStore((s) => s.user?.role)
  const [tasks, setTasks] = useState<DownloadTask[]>([])
  const [torrents, setTorrents] = useState<QBitTorrent[] | null>(null)
  const [url, setURL] = useState('')
  const [savePath, setSavePath] = useState('')

  const refresh = () =>
    downloadsAPI.list().then((d) => {
      setTasks(d.tasks)
      setTorrents(d.torrents)
    })

  useEffect(() => {
    void refresh().catch(() => undefined)
    const id = window.setInterval(() => void refresh().catch(() => undefined), 5_000)
    return () => window.clearInterval(id)
  }, [])

  const onAdd = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await downloadsAPI.add(url, savePath)
      toast.success('已加入下载队列')
      setURL('')
      setSavePath('')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '提交失败'
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <h1 className="font-display text-3xl font-bold text-ink-600">下载管理</h1>
        <p className="flex items-center gap-2 text-sm text-ink-50">
          <ShieldCheck size={16} className="text-brand-500" />
          页面仅展示安全标题、海报和进度信息，不再暴露种子原始 URL 或私有 Token。
        </p>
      </header>

      <form onSubmit={onAdd} className="glass-panel grid gap-3 md:grid-cols-[1fr_1fr_auto]">
        <input
          required
          className="input-base md:col-span-2"
          placeholder="磁力链接 / .torrent URL（提交后不会在页面公开显示）"
          value={url}
          onChange={(e) => setURL(e.target.value)}
        />
        <input
          className="input-base"
          placeholder="保存路径 (可选)"
          value={savePath}
          onChange={(e) => setSavePath(e.target.value)}
        />
        <button type="submit" className="neon-button md:col-span-3">
          <Download size={16} /> 添加
        </button>
      </form>

      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-display text-xl font-semibold text-ink-600">当前下载</h2>
          <span className="text-xs text-ink-50">每 5 秒自动刷新</span>
        </div>
        {torrents === null && (
          <div className="glass-panel text-sand-500">
            尚未连接到下载器 —{' '}
            {role === 'admin' ? (
              <>
                请到{' '}
                <Link to="/download-clients" className="text-brand-500 hover:underline">
                  下载器
                </Link>{' '}
                页面添加并测试连接。
              </>
            ) : (
              '请联系管理员添加并测试下载器连接。'
            )}
          </div>
        )}
        {torrents && torrents.length === 0 && (
          <div className="glass-panel text-sand-500">暂无运行中任务。</div>
        )}
        {torrents && torrents.length > 0 && (
          <div className="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">
            {torrents.map((torrent) => (
              <DownloadTaskCard
                key={torrent.hash}
                item={toLiveCard(torrent)}
                removable={role === 'admin'}
                onRemove={async () => {
                  if (!(await confirmAction({ title: '删除下载任务', message: `删除「${torrent.title || torrent.name}」?`, confirmText: '删除' }))) return
                  await downloadsAPI.remove(torrent.hash, false)
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
        {tasks.length === 0 ? (
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
  )
}
