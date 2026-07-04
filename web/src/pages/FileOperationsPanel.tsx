import { Copy, GitBranch, Move, Pencil, Plus, Trash2 } from 'lucide-react'

import type { FileEntry } from '../api/files'

type FileOperationsPanelProps = {
  selected: FileEntry | null
  folderName: string
  renameTo: string
  destPath: string
  transferMode: string
  busy: string
  onFolderNameChange: (value: string) => void
  onRenameToChange: (value: string) => void
  onDestPathChange: (value: string) => void
  onTransferModeChange: (value: string) => void
  onCreateFolder: () => void
  onRenameSelected: () => void
  onDeleteSelected: () => void
  onTransferSelected: () => void
}

export function FileOperationsPanel({
  selected,
  folderName,
  renameTo,
  destPath,
  transferMode,
  busy,
  onFolderNameChange,
  onRenameToChange,
  onDestPathChange,
  onTransferModeChange,
  onCreateFolder,
  onRenameSelected,
  onDeleteSelected,
  onTransferSelected,
}: FileOperationsPanelProps) {
  return (
    <details className="glass-panel" open={Boolean(selected)}>
      <summary className="cursor-pointer list-none font-display text-lg font-semibold text-ink-600">
        文件操作
        <span className="ml-2 text-xs font-normal text-sand-500">
          新建目录 / 重命名 / 删除 / 转移
        </span>
      </summary>
      <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_1.2fr]">
        <div className="space-y-2">
          <h2 className="text-sm font-semibold text-ink-600">新建目录</h2>
          <div className="flex gap-2">
            <input
              className="input-base flex-1"
              placeholder="新目录名称"
              value={folderName}
              onChange={(event) => onFolderNameChange(event.target.value)}
            />
            <button className="neon-button" disabled={busy === 'mkdir' || !folderName.trim()} onClick={onCreateFolder}>
              <Plus size={16} /> 创建
            </button>
          </div>
        </div>

        <div className="space-y-2">
          <h2 className="text-sm font-semibold text-ink-600">选中项</h2>
          {selected ? (
            <div className="space-y-2">
              <p className="truncate font-mono text-xs text-ink-50" title={selected.path}>{selected.path}</p>
              <div className="flex flex-wrap gap-2">
                <input
                  className="input-base min-w-[220px] flex-1"
                  value={renameTo}
                  onChange={(event) => onRenameToChange(event.target.value)}
                />
                <button className="neon-button" disabled={busy === 'rename' || !renameTo.trim()} onClick={onRenameSelected}>
                  <Pencil size={16} /> 重命名
                </button>
                <button
                  className="rounded-xl border border-red-400/40 px-3 py-2 text-sm text-red-500 hover:bg-red-50"
                  disabled={busy === 'delete'}
                  onClick={onDeleteSelected}
                >
                  <Trash2 size={16} className="inline" /> 删除
                </button>
              </div>
              {!selected.is_dir && (
                <div className="grid gap-2 md:grid-cols-[1fr_140px_auto]">
                  <input
                    className="input-base"
                    placeholder="目标目录路径"
                    value={destPath}
                    onChange={(event) => onDestPathChange(event.target.value)}
                  />
                  <select
                    className="input-base"
                    value={transferMode}
                    onChange={(event) => onTransferModeChange(event.target.value)}
                  >
                    <option value="copy">复制</option>
                    <option value="move">移动</option>
                    <option value="hardlink">硬链接</option>
                    <option value="symlink">软链接</option>
                  </select>
                  <button className="neon-button" disabled={busy === 'transfer' || !destPath.trim()} onClick={onTransferSelected}>
                    {transferIcon(transferMode)}
                    转移
                  </button>
                </div>
              )}
            </div>
          ) : (
            <p className="text-sm text-ink-50">先在下方列表点击“操作”选择文件或目录。</p>
          )}
        </div>
      </div>
    </details>
  )
}

function transferIcon(mode: string) {
  if (mode === 'move') return <Move size={16} />
  if (mode === 'copy') return <Copy size={16} />
  return <GitBranch size={16} />
}
