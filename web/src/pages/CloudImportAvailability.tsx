import { useResourceImportCapability } from '../hooks/useResourceImportCapability'

export function CloudImportAvailability({ state, retry }: ReturnType<typeof useResourceImportCapability>) {
  if (state === 'enabled') return null
  return <div className="flex flex-wrap items-center gap-2 text-sm text-sand-500">
    <p>{state === 'loading' ? '正在检查网盘入库状态…' : state === 'disabled' ? '网盘入库尚未配置，请使用 PT 搜索 / 订阅下载到本地磁盘。' : '无法获取网盘入库状态，PT 搜索和本地下载仍可使用。'}</p>
    {state === 'error' && <button type="button" className="btn-outline" onClick={retry}>重试</button>}
  </div>
}
