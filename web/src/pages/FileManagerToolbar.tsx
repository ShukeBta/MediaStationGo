import { RefreshCw } from 'lucide-react'

type FileManagerToolbarProps = {
  currentPath?: string
  recursive: boolean
  onRefresh: () => void
  onRecursiveChange: (value: boolean) => void
}

export function FileManagerToolbar({
  currentPath,
  recursive,
  onRefresh,
  onRecursiveChange,
}: FileManagerToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <button className="neon-button !px-3 !py-1 !text-xs" onClick={onRefresh}>
        <RefreshCw size={14} /> 刷新
      </button>
      <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-ink-100">
        <input type="checkbox" checked={recursive} onChange={(e) => onRecursiveChange(e.target.checked)} />
        递归扫描
      </label>
      {currentPath && (
        <span className="rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-ink-100">
          {currentPath}
        </span>
      )}
    </div>
  )
}
