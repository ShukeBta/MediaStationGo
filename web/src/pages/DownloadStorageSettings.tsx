import { type FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { adminAPI } from '../api/admin'
import { apiErrorMessage } from './downloadClientPageModel'

export function DownloadStorageSettings() {
  const [savePath, setSavePath] = useState('')
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [saving, setSaving] = useState(false)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    setState('loading')
    adminAPI.listSettings().then((settings) => {
      if (!active) return
      setSavePath(settings.find((setting) => setting.key === 'qbittorrent.savepath')?.value || '')
      setState('ready')
    }).catch(() => { if (active) setState('error') })
    return () => { active = false }
  }, [attempt])

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (saving || state !== 'ready') return
    setSaving(true)
    try {
      await adminAPI.updateSetting('qbittorrent.savepath', savePath.trim())
      setSavePath(savePath.trim())
      toast.success('已保存全局下载目录，后续新增任务生效')
    } catch (error) { toast.error(apiErrorMessage(error, '保存下载目录失败')) }
    finally { setSaving(false) }
  }

  return <form className="glass-panel space-y-3" onSubmit={save}>
    <h2 className="text-lg font-semibold text-ink-600">本地下载存储位置</h2>
    <p className="text-sm text-sand-500">此全局目录用于未单独指定保存路径的新任务，支持上方配置的默认下载器。</p>
    {state === 'error' ? <div className="flex items-center gap-3"><p role="alert" className="text-sm text-red-500">下载目录设置加载失败</p><button type="button" className="btn-outline" onClick={() => setAttempt((value) => value + 1)}>重试</button></div> : <>
      <label className="block text-sm text-ink-600">默认下载根目录（下载器内路径）
        <input className="input-base mt-2" placeholder="例如 /downloads；留空沿用部署配置或下载器默认目录" value={savePath} disabled={state !== 'ready' || saving} onChange={(event) => setSavePath(event.target.value)} />
      </label>
      <p className="text-xs leading-5 text-sand-500">Docker 中填写下载器容器内的路径。例如将 NAS 上的下载文件夹挂载为 /downloads，这里填写 /downloads。订阅可单独覆盖根目录；智能分类开启时会追加分类子目录。清空后沿用部署配置，再回退到下载器默认目录。修改不会移动已有文件。</p>
      <p className="text-xs leading-5 text-sand-500">下载后的整理入库需要 MediaStationGo 也能访问下载文件夹，并正确配置媒体库挂载。</p>
      <button type="submit" className="btn-primary" disabled={state !== 'ready' || saving}>{saving ? '保存中…' : '保存下载目录'}</button>
    </>}
  </form>
}
