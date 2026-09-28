import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Media } from '../types'
import { STRMDeleteDialog } from '../components/STRMDeleteDialog'

export function MediaSTRMTargetPanel({ media }: { media: Media }) {
  const [target, setTarget] = useState('')
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState(false)
  const isSTRM = Boolean(media.path?.toLowerCase().endsWith('.strm'))
  useEffect(() => {
    let active = true
    setTarget('')
    setError('')
    if (isSTRM) {
      api.get<{ target: string }>(`/media/${media.id}/strm-target`).then((response) => {
        if (active) setTarget(response.data.target)
      }).catch(() => { if (active) setError('无法读取 STRM 文件，请检查文件是否存在。') })
    }
    return () => { active = false }
  }, [isSTRM, media.id, media.updated_at])
  if (!isSTRM) return null
  return <section className="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-white/10">
    <h2 className="text-sm font-bold">STRM 实际目标</h2>
    <p className="break-all font-mono text-xs">{target || error || '读取中…'}</p>
    {target ? <button type="button" className="btn-outline text-red-600" onClick={() => setDeleting(true)}>预览并删除目标</button> : null}
    {deleting ? <STRMDeleteDialog key={media.id} mediaID={media.id} onClose={() => setDeleting(false)} /> : null}
  </section>
}
