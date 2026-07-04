import { useState } from 'react'
import { Copy, Loader2, Move, Upload } from 'lucide-react'
import toast from 'react-hot-toast'

import { storageAPI, type StorageType } from '../api/storage_config'
import { TRANSFER_SUPPORTED_TYPES, fmtBytes } from './storageConfigModel'

interface StorageUploadPanelProps {
  type: StorageType
  transferEnabled: boolean
  transferMode: 'copy' | 'move'
}

export function StorageUploadPanel({ type, transferEnabled, transferMode }: StorageUploadPanelProps) {
  const [sourcePath, setSourcePath] = useState('')
  const [destPath, setDestPath] = useState('/MediaStationGo')
  const [recursive, setRecursive] = useState(true)
  const [includeSidecars, setIncludeSidecars] = useState(true)
  const [overwrite, setOverwrite] = useState(false)
  const [busy, setBusy] = useState(false)

  const supported = TRANSFER_SUPPORTED_TYPES.has(type)

  const submit = async () => {
    if (!supported) {
      toast.error('本地直传目前支持 OpenList / Alist / WebDAV / CloudDrive2。115 建议通过 OpenList、CloudDrive2 或 Alist 桥接后转存。')
      return
    }
    if (!sourcePath.trim()) {
      toast.error('请填写本地源目录或文件路径')
      return
    }
    if (!transferEnabled) {
      toast.error('请先开启“允许转存写入”并保存该外部存储配置')
      return
    }
    setBusy(true)
    try {
      const { result, error } = await storageAPI.uploadLocal(type, {
        source_path: sourcePath.trim(),
        dest_path: destPath.trim() || '/',
        recursive,
        include_sidecars: includeSidecars,
        overwrite,
        transfer_mode: transferMode,
      })
      const errText = error || (result.errors && result.errors.length > 0 ? ` · 错误 ${result.errors.length}` : '')
      const movedText = result.moved ? ` · 移动 ${result.moved}` : ''
      toast.success(`转存完成：上传 ${result.uploaded}${movedText} · 跳过 ${result.skipped} · ${fmtBytes(result.bytes)}${errText}`)
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '转存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h3 className="flex items-center gap-2 font-display text-base font-semibold text-[var(--app-text)]">
            {transferMode === 'move' ? <Move size={16} /> : <Copy size={16} />} 本地媒体转存到此存储
          </h3>
          <p className="mt-1 text-xs text-[var(--app-muted)]">
            {transferMode === 'move'
              ? '移动模式会先上传到外部存储，上传成功后才删除本地源文件；远端已存在且未覆盖时不会删除本地。'
              : '复制本地媒体文件到外部存储，保留本地源文件；自动跳过远端已存在文件。'}
          </p>
        </div>
        {!supported && (
          <span className="rounded-full border border-amber-300/50 bg-amber-500/10 px-2 py-1 text-xs text-amber-600">
            直传待接
          </span>
        )}
      </div>
      {!supported && (
        <p className="mb-3 rounded-lg border border-amber-300/50 bg-amber-500/10 px-3 py-2 text-xs text-amber-600">
          115 原生上传需要私有分片上传协议。推荐把 115、123、阿里等网盘挂载到 OpenList、CloudDrive2 或 Alist 后，在这里选择 OpenList / CloudDrive2 / Alist 转存。
        </p>
      )}
      {type === 'openlist' && (
        <p className="mb-3 rounded-lg border border-blue-300/50 bg-blue-500/10 px-3 py-2 text-xs text-blue-500">
          OpenList 优先使用服务地址 + 用户名密码/Token 调用 API 进行浏览、挂载、转存和获取播放直链；WebDAV URL 只是兼容备用。默认端口常见为 5244，未配置 HTTPS 反代时请填写 http://。
        </p>
      )}
      {type === 'clouddrive2' && (
        <p className="mb-3 rounded-lg border border-blue-300/50 bg-blue-500/10 px-3 py-2 text-xs text-blue-500">
          CloudDrive2 已经对接 115、123、阿里等网盘；这里通过它的 WebDAV 入口浏览、挂载和上传，播放默认走服务端反代以携带认证头。
        </p>
      )}
      <div className="grid gap-3 lg:grid-cols-2">
        <label className="block">
          <span className="mb-1 block text-sm text-[var(--app-subtle)]">本地源目录 / 文件</span>
          <input
            className="input-base"
            placeholder="例如 /media/电影 或 F:\\media\\Movies"
            value={sourcePath}
            onChange={(event) => setSourcePath(event.target.value)}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-[var(--app-subtle)]">网盘目标目录</span>
          <input
            className="input-base"
            placeholder="/MediaStationGo"
            value={destPath}
            onChange={(event) => setDestPath(event.target.value)}
          />
        </label>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-[var(--app-subtle)]">
        <label className="flex items-center gap-2">
          <input type="checkbox" className="h-4 w-4 accent-primary-400" checked={recursive} onChange={(event) => setRecursive(event.target.checked)} />
          递归目录
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" className="h-4 w-4 accent-primary-400" checked={includeSidecars} onChange={(event) => setIncludeSidecars(event.target.checked)} />
          同步 NFO / 海报 / 字幕
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" className="h-4 w-4 accent-primary-400" checked={overwrite} onChange={(event) => setOverwrite(event.target.checked)} />
          覆盖远端同名文件
        </label>
        <button type="button" className="neon-button ml-auto" disabled={busy || !supported || !transferEnabled} onClick={submit}>
          {busy ? <Loader2 size={16} className="animate-spin" /> : transferMode === 'move' ? <Move size={16} /> : <Upload size={16} />}
          {busy ? '转存中…' : transferMode === 'move' ? '开始移动转存' : '开始复制转存'}
        </button>
      </div>
    </div>
  )
}
