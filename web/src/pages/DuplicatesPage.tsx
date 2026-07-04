import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Copy, Trash2 } from 'lucide-react'

import { duplicatesAPI, type DuplicateReport } from '../api/duplicates'
import { libraryAPI } from '../api/library'
import { confirmAction } from '../components/confirmAction'
import type { Library } from '../types'

function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${u[i]}`
}

export function DuplicatesPage() {
  const [libs, setLibs] = useState<Library[]>([])
  const [libID, setLibID] = useState('')
  const [report, setReport] = useState<DuplicateReport | null>(null)
  const [scanning, setScanning] = useState(false)

  useEffect(() => {
    libraryAPI.list().then(setLibs)
  }, [])

  useEffect(() => {
    duplicatesAPI.list(libID).then(setReport).catch(() => setReport(null))
  }, [libID])

  const scan = async () => {
    setScanning(true)
    try {
      const r = await duplicatesAPI.scan(libID)
      setReport(r)
      const cleaned = r.missing_removed ? `, 清理 ${r.missing_removed} 条失效记录` : ''
      toast.success(`扫描完成: ${r.groups_found} 组重复, ${r.items_marked} 项标记${cleaned}`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '扫描失败'
      toast.error(msg)
    } finally {
      setScanning(false)
    }
  }

  const unmark = async () => {
    if (!(await confirmAction({ title: '清除重复标记', message: '清除所有重复标记?(磁盘文件不会被删除)', confirmText: '清除' }))) return
    const r = await duplicatesAPI.unmark(libID)
    toast.success(`已清除 ${r.unmarked} 项`)
    setReport(null)
  }

  return (
    <div className="space-y-6">
      <header className="flex items-center gap-3">
        <Copy className="h-6 w-6 text-brand-500" />
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">重复文件</h1>
          <p className="text-sm text-ink-50">
            通过稀疏采样 MD5(头部 / 中部 / 尾部各 1 MiB + 文件大小)检测重复媒体,
            同一组中保留刮削过的较大文件作为主条目,其余标记为重复。
          </p>
        </div>
      </header>

      <div className="glass-panel grid gap-3 md:grid-cols-[1fr_auto_auto]">
        <select
          className="input-base"
          value={libID}
          onChange={(e) => setLibID(e.target.value)}
        >
          <option value="">所有媒体库</option>
          {libs.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name}
            </option>
          ))}
        </select>
        <button onClick={scan} disabled={scanning} className="neon-button">
          {scanning ? '扫描中…' : '开始扫描'}
        </button>
        <button onClick={unmark} className="neon-button !border-red-400/40 !text-red-400">
          <Trash2 size={14} /> 清除标记
        </button>
      </div>

      {report && report.groups_found === 0 && (
        <p className="text-ink-50">扫描了 {report.total_scanned} 项,未发现重复。</p>
      )}

      {report && report.missing_removed ? (
        <p className="rounded-2xl border border-amber-300/50 bg-amber-50 px-4 py-3 text-sm text-amber-700">
          已清理 {report.missing_removed} 条文件不存在的媒体记录，统计容量会在刷新后恢复正常。
        </p>
      ) : null}

      {report && (report.groups ?? []).map((g) => (
        <section key={g.hash} className="glass-panel space-y-2">
          <div className="flex items-center justify-between">
            <p className="font-mono text-xs text-sand-500">{g.hash}</p>
            <span className="rounded-lg border border-emerald-400/40 px-2 py-0.5 text-xs text-emerald-400">
              主条目
            </span>
          </div>
          <p className="font-medium text-ink-600">{g.primary.title}</p>
          <p className="font-mono text-xs text-ink-50">
            {g.primary.path} · {fmtBytes(g.primary.size_bytes)}
          </p>
          <div className="space-y-1 border-t border-gray-200 pt-2">
            <p className="text-xs uppercase tracking-wider text-red-400">
              重复 ({g.duplicates.length})
            </p>
            {g.duplicates.map((d) => (
              <div key={d.id} className="text-xs text-ink-50">
                <span className="font-medium text-ink-600">{d.title}</span> · {d.path} · {fmtBytes(d.size_bytes)}
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
