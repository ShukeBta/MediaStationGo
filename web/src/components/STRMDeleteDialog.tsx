import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { api } from '../api/client'

type DeleteTarget = { target_path: string; parent_path?: string; confirmation: string }

export function STRMDeleteDialog({ mediaID, onClose, onDeleted }: { mediaID: string; onClose: () => void; onDeleted?: () => void }) {
  const [target, setTarget] = useState<DeleteTarget | null>(null)
  const [error, setError] = useState('')
  const [deleteParent, setDeleteParent] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    let active = true
    api.get<DeleteTarget>(`/media/${mediaID}/strm-delete-target`).then((response) => {
      if (active) setTarget(response.data)
    }).catch(() => {
      if (active) setError('目标不是可信目录中的本地媒体文件，或文件已不存在。请检查 STRM 内容和路径映射。')
    })
    return () => { active = false }
  }, [mediaID])
  const remove = async () => {
    if (!target || !confirmed || busy) return
    setBusy(true)
    try {
      await api.delete(`/media/${mediaID}/strm-target`, { data: { delete_parent: deleteParent, confirmation: target.confirmation } })
      toast.success('目标已删除，STRM 引用和媒体库记录保留。')
      onDeleted?.()
      onClose()
    } catch {
      setError('删除未完成。目标可能已变化，请关闭窗口后重新预览；也可检查文件权限。')
      setConfirmed(false)
    } finally { setBusy(false) }
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => { if (!busy) onClose() }}>
      <section role="dialog" aria-modal="true" aria-labelledby="strm-delete-title" className="w-full max-w-xl rounded-2xl bg-white p-6 shadow-xl dark:bg-gray-900" onClick={(event) => event.stopPropagation()}>
        <h2 id="strm-delete-title" className="text-lg font-bold">删除 STRM 指向的文件</h2>
        {error ? <p role="alert" className="mt-4 text-sm text-red-600">{error}</p> : null}
        {!target && !error ? <p className="mt-4 text-sm">正在读取实际目标…</p> : null}
        {target ? <div className="mt-4 space-y-4 text-sm">
          <p className="break-all rounded-lg bg-gray-100 p-3 font-mono dark:bg-gray-800">{deleteParent ? target.parent_path : target.target_path}</p>
          {target.parent_path ? <label className="flex gap-2"><input type="checkbox" checked={deleteParent} onChange={(event) => { setDeleteParent(event.target.checked); setConfirmed(false) }} />同时删除父目录及其中的全部文件</label> : null}
          <p className="text-red-600">此操作会永久删除{deleteParent ? '上方目录及其全部内容' : '上方媒体文件'}，无法通过媒体库回收站恢复。STRM 引用和库记录保留。</p>
          <label className="flex gap-2"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} />我已核对路径，确认删除{deleteParent ? '整个目录' : '该文件'}</label>
        </div> : null}
        <div className="mt-6 flex justify-end gap-3">
          <button type="button" disabled={busy} className="btn-outline" onClick={onClose}>取消</button>
          <button type="button" disabled={!target || !confirmed || busy} className="rounded-lg bg-red-600 px-4 py-2 text-white disabled:opacity-40" onClick={() => void remove()}>{busy ? '正在删除…' : '永久删除'}</button>
        </div>
      </section>
    </div>
  )
}
