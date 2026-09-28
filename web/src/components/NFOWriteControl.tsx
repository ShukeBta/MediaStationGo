import { useEffect, useState } from 'react'
import { api } from '../api/client'

export function NFOWriteControl({ mediaID, scope, checked, onChange }: { mediaID: string; scope: 'media' | 'series'; checked: boolean; onChange: (value: boolean) => void }) {
  const [target, setTarget] = useState<{ path: string; exists: boolean } | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    onChange(false)
    setTarget(null)
    setError('')
    api.get<{ path: string; exists: boolean }>(`/media/${mediaID}/nfo-target`, { params: { scope } }).then(({ data }) => {
      if (active) { setTarget(data); onChange(data.exists) }
    }).catch(() => { if (active) setError('本地 NFO 暂不可写，可仅保存媒体库元数据。') })
    return () => { active = false }
  }, [mediaID, scope, onChange])
  return <div className="md:col-span-2 rounded-xl border border-gray-200 p-3 text-sm">
    <label className="flex items-center gap-2 font-semibold"><input type="checkbox" disabled={!target} checked={checked} onChange={(event) => onChange(event.target.checked)} />同步写入本地 NFO</label>
    <p className="mt-2 break-all text-xs text-gray-500">{target ? `${target.exists ? '更新' : '创建'}：${target.path}` : error || '正在检查本地 NFO…'}</p>
    {target && <p className="mt-1 text-xs text-gray-500">保留未编辑的演员角色、轨道及自定义标签。{scope === 'series' ? '整剧编辑同步 tvshow.nfo。' : ''}</p>}
  </div>
}
