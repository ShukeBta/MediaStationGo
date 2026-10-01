import { useEffect, useState } from 'react'
import { Plus } from 'lucide-react'

import { resourceImportsAPI } from '../api/resourceImports'
import { ManualResourceTaskDialog } from './ManualResourceTaskDialog'
import { ResourceImportTasksSection } from './ResourceImportTasksSection'

// Cloud imports are optional. Do not mount their polling list or creation
// dialog until the server confirms that the feature is available.
export function CloudImportDownloadsSection({ isAdmin }: { isAdmin: boolean }) {
  const [state, setState] = useState<'loading' | 'enabled' | 'disabled' | 'error'>('loading')
  const [open, setOpen] = useState(false)
  const [manualTaskOpen, setManualTaskOpen] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setState('loading')
    resourceImportsAPI.capabilities(controller.signal)
      .then(({ enabled }) => { if (!controller.signal.aborted) setState(enabled ? 'enabled' : 'disabled') })
      .catch(() => { if (!controller.signal.aborted) setState('error') })
    return () => controller.abort()
  }, [retry])

  return <details className="border-t border-gray-200 pt-5" onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary className="cursor-pointer select-none text-sm font-semibold text-sand-500 hover:text-ink-600">网盘入库（可选）</summary>
    {open && <div className="mt-5 space-y-4">
      {state === 'loading' && <p className="text-sm text-sand-500">正在检查网盘入库状态…</p>}
      {state === 'disabled' && <p className="text-sm text-sand-500">网盘入库尚未配置。PT 搜索、自动追更和下载器任务不需要开启此功能。</p>}
      {state === 'error' && <div className="flex items-center gap-3 text-sm text-sand-500">
        <p role="alert">无法获取网盘入库状态，请稍后重试。</p>
        <button type="button" className="btn-outline" onClick={() => setRetry((value) => value + 1)}>重试</button>
      </div>}
      {state === 'enabled' && <>
        <button type="button" className="btn-outline px-4 py-2.5" onClick={() => setManualTaskOpen(true)}><Plus size={17} />新建网盘入库任务</button>
        <ResourceImportTasksSection isAdmin={isAdmin} refreshKey={refreshKey} />
        {manualTaskOpen && <ManualResourceTaskDialog onClose={() => setManualTaskOpen(false)} onCreated={() => setRefreshKey((value) => value + 1)} />}
      </>}
    </div>}
  </details>
}
