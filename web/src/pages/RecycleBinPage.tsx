import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Home, RotateCcw, Square, CheckSquare, Trash2 } from 'lucide-react'

import { recycleAPI, type RecycleBinItem } from '../api/recycle'
import { confirmAction } from '../components/confirmAction'
import { formatSize } from './libraryPageModel'

type PurgeProgress = {
  completed: number
  total: number
  failed: number
  currentTitle: string
}

export function RecycleBinPage() {
  const [items, setItems] = useState<RecycleBinItem[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [batchBusy, setBatchBusy] = useState('')
  const [purgeProgress, setPurgeProgress] = useState<PurgeProgress | null>(null)

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
      message: `彻底删除选中的 ${selectedIds.length} 条记录？云盘媒体会同步删除对应源文件，本地磁盘文件仍保留。`,
      confirmText: '彻底删除',
    }))) return
    setBatchBusy(mode)
    try {
      const result = mode === 'purge'
        ? await purgeWithProgress(selectedIds)
        : await runRestoreBatchRequest(selectedIds)
      toast.success(`${mode === 'restore' ? '恢复' : '彻底删除'}完成：${result.applied} 条`)
      setSelectedIds(result.failedIDs)
      if (result.errors.length > 0) toast.error(`${result.errors.length} 条操作失败，已保留选中状态`)
      await refresh()
    } catch (err: unknown) {
      const response = (err as { response?: { status?: number; data?: { error?: string; failed_ids?: string[] } }; message?: string })?.response
      if (response?.data?.failed_ids) setSelectedIds(response.data.failed_ids)
      const msg = response?.data?.error || (response?.status ? `批量操作失败 (${response.status})` : (err as { message?: string })?.message || '批量操作失败')
      toast.error(msg)
    } finally {
      setBatchBusy('')
      setPurgeProgress(null)
    }
  }

  const runRestoreBatchRequest = async (ids: string[]) => {
    try {
      const result = await recycleAPI.restoreMany(ids)
      return { ...result, errors: result.errors ?? [], failedIDs: result.failed_ids ?? [] }
    } catch (err: unknown) {
      const status = (err as { response?: { status?: number } })?.response?.status
      if (status !== 404 && status !== 405) throw err
      let applied = 0
      for (const id of ids) {
        await recycleAPI.restore(id)
        applied += 1
      }
      return { applied, errors: [] as string[], failedIDs: [] as string[] }
    }
  }

  const purgeWithProgress = async (ids: string[]) => {
    let applied = 0
    const failedIDs: string[] = []
    const errors: string[] = []
    const titleByID = new Map(items.map((item) => [item.id, item.title]))

    setPurgeProgress({ completed: 0, total: ids.length, failed: 0, currentTitle: '' })
    for (let index = 0; index < ids.length; index += 1) {
      const id = ids[index]
      const currentTitle = titleByID.get(id) || id
      setPurgeProgress({ completed: index, total: ids.length, failed: failedIDs.length, currentTitle })
      try {
        await recycleAPI.purge(id)
        applied += 1
      } catch (error: unknown) {
        failedIDs.push(id)
        const response = (error as { response?: { status?: number; data?: { error?: string } }; message?: string })?.response
        const message = response?.data?.error
          || (response?.status ? `彻底删除失败 (${response.status})` : (error as { message?: string })?.message)
          || '彻底删除失败'
        errors.push(`${currentTitle}: ${message}`)
      }
      setPurgeProgress({ completed: index + 1, total: ids.length, failed: failedIDs.length, currentTitle })
    }
    return { applied, errors, failedIDs }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">回收站</h1>
          <p className="mt-2 text-sm text-ink-50">
            软删除的媒体可以恢复。彻底删除云盘媒体时会同步删除对应源文件；本地磁盘文件仍保留。
          </p>
          <p className="mt-1 text-xs text-sand-500">本地媒体最多展示最新 200 条；云盘记录会保留到手动恢复或彻底删除。</p>
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
          {purgeProgress && (
            <div className="mb-4 rounded-lg border border-[var(--app-brand-border)] bg-[var(--app-brand-soft)] p-3" role="status" aria-live="polite">
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-[var(--app-text)]">
                <span className="min-w-0 truncate">正在彻底删除：{purgeProgress.currentTitle || '准备中'}</span>
                <span className="flex shrink-0 items-center gap-2 tabular-nums text-[var(--app-muted)]">
                  <span>已完成 {purgeProgress.completed} / {purgeProgress.total}</span>
                  {purgeProgress.failed > 0 && <span className="text-red-500">失败 {purgeProgress.failed}</span>}
                </span>
              </div>
              <div className="mt-2 h-1.5 overflow-hidden rounded bg-[var(--app-border)]">
                <div
                  className="h-full rounded bg-brand-500 transition-[width] duration-200"
                  style={{ width: `${purgeProgress.total > 0 ? Math.round((purgeProgress.completed / purgeProgress.total) * 100) : 0}%` }}
                />
              </div>
            </div>
          )}
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="w-10 py-2">
                  <button onClick={toggleAll} className="text-sand-500 hover:text-brand-500" aria-label="全选">
                    {allSelected ? <CheckSquare size={16} /> : <Square size={16} />}
                  </button>
                </th>
                <th className="py-2">标题</th>
                <th>版本与路径</th>
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
                  <td className="py-2 text-ink-600">
                    <div>{m.title}</div>
                    <div className="mt-0.5 text-xs text-sand-500">
                      {m.deletion_kind === 'version' ? '片源版本' : '媒体记录'} · {new Date(m.deleted_at).toLocaleString()}
                    </div>
                  </td>
                  <td className="max-w-md text-ink-50">
                    <div>{[m.width && m.height ? `${m.width}x${m.height}` : '', m.container?.toUpperCase(), m.size_bytes ? formatSize(m.size_bytes) : ''].filter(Boolean).join(' · ') || '未探测版本'}</div>
                    {m.path && <div className="truncate text-xs text-sand-500" title={m.path}>{m.path}</div>}
                  </td>
                  <td className="space-x-2 py-2 text-right">
                    <button
                      type="button"
                      title="恢复这条记录"
                      aria-label={`恢复 ${m.title}`}
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
                      type="button"
                      title="彻底删除这条记录"
                      aria-label={`彻底删除 ${m.title}`}
                      className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                      onClick={async () => {
                        if (!(await confirmAction({ title: '彻底删除记录', message: `彻底删除「${m.title}」？云盘媒体会同步删除对应源文件，本地磁盘文件仍保留。`, confirmText: '彻底删除' }))) return
                        setBatchBusy('purge')
                        try {
                          const result = await purgeWithProgress([m.id])
                          if (result.applied > 0) toast.success('已彻底删除')
                          if (result.errors.length > 0) toast.error(result.errors.join('\n'))
                          setSelectedIds(result.failedIDs)
                          await refresh()
                        } catch (err: unknown) {
                          const response = (err as { response?: { data?: { error?: string; failed_ids?: string[] }; status?: number }; message?: string })?.response
                          if (response?.data?.failed_ids) setSelectedIds(response.data.failed_ids)
                          toast.error(response?.data?.error || (response?.status ? `彻底删除失败 (${response.status})` : (err as { message?: string })?.message || '彻底删除失败'))
                        } finally {
                          setBatchBusy('')
                        }
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
