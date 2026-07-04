import { Loader2, PauseCircle, RefreshCw } from 'lucide-react'

import type { CloudScanStatus } from '../api/storage_config'

interface CloudScanPanelProps {
  scanBusy: boolean
  cancelBusy: boolean
  scanStatuses: CloudScanStatus[]
  onScanAll: () => void
  onCancelScans: () => void
}

export function CloudScanPanel({
  scanBusy,
  cancelBusy,
  scanStatuses,
  onScanAll,
  onCancelScans,
}: CloudScanPanelProps) {
  return (
    <div className="mb-3 rounded border border-emerald-300/30 bg-emerald-500/10 p-2">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-xs font-semibold text-[var(--app-subtle)]">网盘媒体库扫描</div>
          <p className="text-xs text-[var(--app-muted)]">
            只需在系统设置填写公开域名，扫描会自动为网盘媒体生成 STRM/302 播放入口；中断后再次扫描会去重补齐。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className="rounded border border-emerald-300/70 px-2 py-1 text-xs text-emerald-600 hover:bg-emerald-500/10"
            disabled={scanBusy}
            onClick={onScanAll}
          >
            {scanBusy ? <Loader2 size={13} className="inline animate-spin" /> : <RefreshCw size={13} className="inline" />}
            {' '}一键扫描全部网盘库
          </button>
          <button
            type="button"
            className="rounded border border-red-300/60 px-2 py-1 text-xs text-red-500 hover:bg-[var(--app-danger-soft)]"
            disabled={cancelBusy}
            onClick={onCancelScans}
          >
            {cancelBusy ? <Loader2 size={13} className="inline animate-spin" /> : <PauseCircle size={13} className="inline" />}
            {' '}中断当前网盘扫描
          </button>
        </div>
      </div>
      {scanStatuses.length > 0 && (
        <div className="grid gap-1 text-xs text-[var(--app-muted)] md:grid-cols-2">
          {scanStatuses.slice(0, 6).map((item) => (
            <div key={item.library_id} className="rounded border border-[var(--app-border)] bg-[var(--app-panel)] px-2 py-1">
              <span className="font-mono text-[var(--app-subtle)]">{item.state}</span>
              {' · '}
              {item.provider}
              {' · 目录 '}
              {item.dirs}
              {' · 发现 '}
              {item.discovered}
              {' · 入库 '}
              {item.added + item.updated}
              {item.error ? <span className="text-red-500"> · {item.error}</span> : null}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
