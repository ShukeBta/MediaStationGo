import { FileVideo, Folder, Loader2, Pencil } from 'lucide-react'

import type { CloudEntry } from '../api/storage_config'

interface CloudEntryListProps {
  loading: boolean
  error: string
  items: CloudEntry[]
  onEnter: (entry: CloudEntry) => void
  onImport: (entry: CloudEntry) => void
  onRename: (entry: CloudEntry) => void
}

export function CloudEntryList({ loading, error, items, onEnter, onImport, onRename }: CloudEntryListProps) {
  if (loading) {
    return (
      <div className="flex justify-center py-4 text-[var(--app-muted)]">
        <Loader2 className="animate-spin" size={16} />
      </div>
    )
  }

  if (error) return <p className="py-2 text-sm text-red-400">{error}</p>
  if (items.length === 0) return <p className="py-2 text-sm text-[var(--app-muted)]">该目录为空</p>

  return (
    <ul className="divide-y divide-[var(--app-border)]">
      {items.map((entry) => (
        <li key={entry.id} className="flex items-center gap-2 py-1.5 text-sm">
          {entry.is_dir ? <Folder size={15} className="text-amber-400" /> : <FileVideo size={15} className="text-blue-300" />}
          {entry.is_dir ? (
            <>
              <button type="button" className="flex-1 text-left text-[var(--app-subtle)] hover:text-brand-500" onClick={() => onEnter(entry)}>
                {entry.name}
              </button>
              <button
                type="button"
                title="重命名文件夹"
                className="rounded border border-[var(--app-border)] p-1 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
                onClick={() => onRename(entry)}
              >
                <Pencil size={14} />
              </button>
            </>
          ) : (
            <>
              <span className="flex-1 truncate text-[var(--app-subtle)]">{entry.name}</span>
              <button
                type="button"
                className="rounded border border-[var(--app-border)] px-2 py-0.5 text-xs text-[var(--app-subtle)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
                onClick={() => onImport(entry)}
              >
                导入
              </button>
            </>
          )}
        </li>
      ))}
    </ul>
  )
}
