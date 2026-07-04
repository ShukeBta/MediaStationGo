import { FolderOpen } from 'lucide-react'

type FileRoot = {
  label: string
  path: string
}

type FileBrowserRootsProps = {
  roots: FileRoot[]
  onOpen: (path: string) => void
}

export function FileBrowserRoots({ roots, onOpen }: FileBrowserRootsProps) {
  return (
    <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
      {roots.map((root) => (
        <button
          key={root.path}
          onClick={() => onOpen(root.path)}
          className="glass-panel flex items-center gap-3 text-left transition hover:border-primary-400/40"
        >
          <FolderOpen size={20} className="text-brand-500" />
          <div>
            <p className="font-mono text-sm text-ink-600">{root.label}</p>
            <p className="font-mono text-xs text-ink-50">{root.path}</p>
          </div>
        </button>
      ))}
    </div>
  )
}
