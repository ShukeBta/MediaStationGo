import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Home, RotateCcw, Square, CheckSquare, Trash2 } from 'lucide-react'

import { recycleAPI } from '../api/recycle'
import { confirmAction } from '../components/confirmAction'
import type { Media } from '../types'

export function RecycleBinPage() {
  const [items, setItems] = useState<Media[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [batchBusy, setBatchBusy] = useState('')

  const refresh = () =>
    recycleAPI
      .list()
      .then((rows) => {
        setItems(rows)
        setSelectedIds((current) => current.filter((id) => rows.some((item) => item.id === id)))
      })
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const allSelected = items.length > 0 && selectedIds.length === items.length
  const toggleAll = () => {
    setSelectedIds(allSelected ? [] : items.map((item) => item.id))
  }
  const toggleOne = (id: string) => {
    setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id])
  }
  const runBatch = async (mode: 'restore' | 'purge') => {
    if (selectedIds.length === 0) return
    if (mode === 'purge' && !(await confirmAction({
      title: '批量彻底删除记录',
      message: `彻底删除选中的 ${selectedIds.length} 条记录? (磁盘文件保留)`,
      confirmText: '彻底删除',
    }))) return
    setBatchBusy(mode)
    try {
      const result = await runRecycleBatchRequest(mode, selectedIds)
      toast.success(`${mode === 'restore' ? '恢复' : '彻底删除'}完成：${result.applied} 条`)
      setSelectedIds([])
      await refresh()
    } catch (err: unknown) {
      const response = (err as { response?: { status?: number; data?: { error?: string } }; message?: string })?.response
      const msg = response?.data?.error || (response?.status ? `批量操作失败 (${response.status})` : (err as { message?: string })?.message || '批量操作失败')
      toast.error(msg)
    } finally {
      setBatchBusy('')
    }
  }

  const runRecycleBatchRequest = async (mode: 'restore' | 'purge', ids: string[]) => {
    try {
      return mode === 'restore'
        ? await recycleAPI.restoreMany(ids)
        : await recycleAPI.purgeMany(ids)
    } catch (err: unknown) {
      const status = (err as { response?: { status?: number } })?.response?.status
      if (status !== 404 && status !== 405) throw err
      let applied = 0
      for (const id of ids) {
        if (mode === 'restore') {
          await recycleAPI.restore(id)
        } else {
          await recycleAPI.purge(id)
        }
        applied += 1
      }
      return { applied, errors: [] as string[] }
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">回收站</h1>
          <p className="mt-2 text-sm text-ink-50">
            软删除的媒体保留在数据库中,可以恢复。彻底删除不会移除磁盘上的文件,只会从数据库清除条目。
          </p>
          <p className="mt-1 text-xs text-sand-500">系统最多保留最新 200 条回收站记录，超过后会自动清理旧记录。</p>
        </div>
        <Link to="/" className="btn-outline shrink-0 py-2.5 px-4 text-sm">
          <Home size={15} />
          返回系统首页
        </Link>
      </div>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && items.length === 0 && <p className="text-ink-50">回收站为空。</p>}

      {items.length > 0 && (
        <div className="glass-panel">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div className="text-xs font-semibold text-sand-500">
              已选择 {selectedIds.length} / {items.length} 条
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => runBatch('restore')}
                disabled={selectedIds.length === 0 || !!batchBusy}
                className="btn-outline py-2 px-3 text-xs"
              >
                <RotateCcw size={13} />
                {batchBusy === 'restore' ? '恢复中…' : '批量恢复'}
              </button>
              <button
                onClick={() => runBatch('purge')}
                disabled={selectedIds.length === 0 || !!batchBusy}
                className="btn-outline py-2 px-3 text-xs !border-red-100 !text-red-500 hover:!bg-red-50 hover:!border-red-200"
              >
                <Trash2 size={13} />
                {batchBusy === 'purge' ? '删除中…' : '批量彻底删除'}
              </button>
            </div>
          </div>
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="w-10 py-2">
                  <button onClick={toggleAll} className="text-sand-500 hover:text-brand-500" aria-label="全选">
                    {allSelected ? <CheckSquare size={16} /> : <Square size={16} />}
                  </button>
                </th>
                <th className="py-2">标题</th>
                <th>路径</th>
                <th className="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((m) => (
                <tr key={m.id} className="border-t border-gray-200">
                  <td className="py-2">
                    <button onClick={() => toggleOne(m.id)} className="text-sand-500 hover:text-brand-500" aria-label={`选择 ${m.title}`}>
                      {selectedIds.includes(m.id) ? <CheckSquare size={16} /> : <Square size={16} />}
                    </button>
                  </td>
                  <td className="py-2 text-ink-600">{m.title}</td>
                  <td className="max-w-md truncate text-ink-50" title={m.path}>
                    {m.path}
                  </td>
                  <td className="space-x-2 py-2 text-right">
                    <button
                      className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                      onClick={async () => {
                        await recycleAPI.restore(m.id)
                        toast.success('已恢复')
                        await refresh()
                      }}
                    >
                      <RotateCcw size={12} />
                    </button>
                    <button
                      className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                      onClick={async () => {
                        if (!(await confirmAction({ title: '彻底删除记录', message: `彻底删除「${m.title}」? (磁盘文件保留)`, confirmText: '彻底删除' }))) return
                        await recycleAPI.purge(m.id)
                        toast.success('已彻底删除')
                        await refresh()
                      }}
                    >
                      <Trash2 size={12} />
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
