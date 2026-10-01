import { Link } from 'react-router-dom'

export function LocalDownloadPathField({ value, onChange, disabled = false, canManage = false, subscription = false }: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  canManage?: boolean
  subscription?: boolean
}) {
  return <div className="space-y-2 rounded-xl border border-gray-200 bg-gray-50 p-3">
    <label className="block text-sm font-medium text-ink-600">
      {subscription ? '本地下载根目录（下载器内路径）' : '本地下载目录（下载器内路径）'}
      <input className="input-base mt-2" placeholder="留空使用默认目录，例如 /downloads" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
    </label>
    <p className="text-xs leading-5 text-sand-500">文件由默认下载器保存到本地磁盘。Docker 部署请填写 qBittorrent 等下载器容器内的路径，并将该目录挂载到 NAS 磁盘。留空沿用全局目录、部署配置或下载器自身默认目录。</p>
    {subscription && <p className="text-xs text-sand-500">启用智能分类时，订阅会在根目录下追加分类子目录；修改仅影响后续新增任务。</p>}
    {canManage && <Link to="/download-clients" className="inline-block text-xs text-brand-500">配置默认下载器和全局下载目录</Link>}
  </div>
}
