import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Database, HardDrive, PieChart } from 'lucide-react'

import { storageAPI, type StorageBreakdown } from '../api/storage'
import { ManagementShortcuts } from '../components/ManagementShortcuts'

function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(2)} ${u[i]}`
}

function fmtHours(seconds: number): string {
  if (!seconds) return '—'
  const h = Math.floor(seconds / 3600)
  return `${h.toLocaleString()} h`
}

// StoragePage shows disk usage broken down by library and by container.
export function StoragePage() {
  const [data, setData] = useState<StorageBreakdown | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    storageAPI
      .breakdown()
      .then(setData)
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-sand-500">加载中…</p>
  if (!data) return <p className="text-sand-500">无法获取存储数据</p>

  const totalBytes = data.total_bytes || 1

  return (
    <div className="space-y-8">
      <header className="flex items-center gap-3">
        <HardDrive className="h-6 w-6 text-brand-500" />
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">存储与文件</h1>
          <p className="text-sm text-ink-50">
            文件浏览、整理入库、排重与存储统计统一在这里，避免入口重复。
          </p>
        </div>
      </header>

      <ManagementShortcuts
        title="核心操作"
        description="保留常用入口：先整理/入库，再按需进入文件或清理功能。"
        items={[
          { to: '/files', title: '文件管理', description: '浏览服务器文件，做少量安全文件操作', group: '整理入库' },
          { to: '/storage-config', title: '存储配置', description: '维护媒体存储路径和容量策略', group: '整理入库' },
          { to: '/duplicates', title: '重复清理', description: '扫描重复媒体并进行安全清理', badge: '清理', group: '空间维护' },
          { to: '/recycle', title: '回收站', description: '查看已删除资源并执行恢复或释放空间', group: '空间维护' },
        ]}
      />

      <details className="glass-panel group">
        <summary className="cursor-pointer list-none font-display text-lg font-semibold text-ink-600">
          低频维护入口 <span className="text-xs font-normal text-sand-500">（点击展开）</span>
        </summary>
        <div className="mt-3 grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-6">
          <MaintenanceLink to="/strm" title="STRM 生成" />
          <MaintenanceLink to="/scheduler" title="定时任务" />
          <MaintenanceLink to="/tasks" title="任务队列" />
          <MaintenanceLink to="/stats" title="运行状态" />
          <MaintenanceLink to="/notify-channels" title="通知渠道" />
          <MaintenanceLink to="/assistant" title="AI 对话台" />
        </div>
      </details>

      <section className="grid gap-4 sm:grid-cols-3">
        <Tile icon={<Database size={20} />} label="总占用" value={fmtBytes(data.total_bytes)} />
        <Tile icon={<PieChart size={20} />} label="媒体库" value={`${data.by_library.length}`} />
        <Tile icon={<HardDrive size={20} />} label="累计时长" value={fmtHours(data.total_seconds)} />
      </section>

      <section className="space-y-3">
        <h2 className="font-display text-xl font-semibold text-ink-600">按媒体库</h2>
        <div className="glass-panel">
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="py-2">名称</th>
                <th>类型</th>
                <th>媒体数</th>
                <th>占用</th>
                <th>占比</th>
              </tr>
            </thead>
            <tbody>
              {data.by_library.map((l) => {
                const pct = (l.total_bytes / totalBytes) * 100
                return (
                  <tr key={l.library_id} className="border-t border-gray-200">
                    <td className="py-2 text-ink-600">{l.name}</td>
                    <td className="text-ink-100">{l.type}</td>
                    <td className="text-ink-100">{l.media_count}</td>
                    <td className="text-ink-100">{fmtBytes(l.total_bytes)}</td>
                    <td>
                      <div className="flex items-center gap-2">
                        <div className="h-1 w-24 overflow-hidden rounded bg-gray-200">
                          <div
                            className="h-full bg-primary-400"
                            style={{ width: `${pct.toFixed(1)}%` }}
                          />
                        </div>
                        <span className="text-xs text-ink-50">{pct.toFixed(1)}%</span>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </section>

      <section className="space-y-3">
        <h2 className="font-display text-xl font-semibold text-ink-600">按容器格式</h2>
        <div className="grid gap-3 sm:grid-cols-2 md:grid-cols-3">
          {data.by_container.map((c) => (
            <div
              key={c.container}
              className="glass-panel flex items-center justify-between !p-4"
            >
              <div>
                <p className="text-xs uppercase tracking-wider text-sand-500">{c.container}</p>
                <p className="font-display text-lg font-semibold text-ink-600">{c.count} 项</p>
              </div>
              <p className="text-sm text-ink-100">{fmtBytes(c.bytes)}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

function Tile({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode
  label: string
  value: string
}) {
  return (
    <div className="glass-panel flex items-center gap-3 !p-4">
      <div className="rounded-xl border border-primary-400/40 bg-primary-400/10 p-2 text-brand-500">
        {icon}
      </div>
      <div>
        <p className="text-xs uppercase tracking-wider text-sand-500">{label}</p>
        <p className="font-display text-lg font-semibold text-ink-600">{value}</p>
      </div>
    </div>
  )
}

function MaintenanceLink({ to, title }: { to: string; title: string }) {
  return (
    <Link
      to={to}
      className="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-ink-100 transition hover:border-primary-400/40 hover:text-brand-500"
    >
      {title}
    </Link>
  )
}
