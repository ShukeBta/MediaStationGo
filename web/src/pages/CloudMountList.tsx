import { Trash2 } from 'lucide-react'

import type { Library } from '../types'
import { cloudLibraryLabel } from './storageConfigModel'

interface CloudMountListProps {
  mounts: Library[]
  onRemove: (library: Library) => void
}

export function CloudMountList({ mounts, onRemove }: CloudMountListProps) {
  if (mounts.length === 0) return null

  return (
    <div className="mb-3 rounded border border-blue-300/30 bg-blue-500/10 p-2">
      <div className="mb-1 text-xs font-semibold text-[var(--app-subtle)]">已挂载目录</div>
      <div className="space-y-1">
        {mounts.map((lib) => (
          <div key={lib.id} className="flex items-center gap-2 rounded border border-[var(--app-border)] bg-[var(--app-panel)] px-2 py-1 text-xs">
            <span className="min-w-0 flex-1 truncate text-[var(--app-subtle)]">
              {lib.name} · {cloudLibraryLabel(lib.path)}
            </span>
            <button
              type="button"
              className="rounded border border-red-300/60 px-1.5 py-0.5 text-red-500 hover:bg-[var(--app-danger-soft)]"
              onClick={() => onRemove(lib)}
              title="移除挂载"
            >
              <Trash2 size={13} />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
