import type { MouseEvent, ReactNode } from 'react'
import { MoreVertical, Power, PowerOff, RefreshCw, Save, Trash2 } from 'lucide-react'

import type { Library, LibraryRoot } from '../types'
import type { RootDraft } from './adminLibraryPanelModel'
import { displayLibraryRootName, displayLibraryRootPath, fallbackLibraryRoot } from './adminLibraryPanelModel'

type LibraryTableProps = {
  libs: Library[]
  editableRootDraft: (libraryID: string, root: LibraryRoot) => RootDraft
  onEditableRootChange: (libraryID: string, root: LibraryRoot, patch: Partial<RootDraft>) => void
  onSaveRoot: (libraryID: string, root: LibraryRoot) => void
  onScanRoot: (libraryID: string, root: LibraryRoot) => void
  onToggleRoot: (libraryID: string, root: LibraryRoot) => void
  onRemoveRoot: (library: Library, root: LibraryRoot) => void
  onScanLibrary: (library: Library) => void
  onRemoveLibrary: (library: Library) => void
}

export function AdminLibraryTable({ libs, ...actions }: LibraryTableProps) {
  return (
    <div className="glass-panel overflow-x-auto !p-3">
      <table className="w-full min-w-[900px] text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="w-28 py-2">名称</th>
            <th>路径</th>
            <th className="w-20">类型</th>
            <th className="w-12 text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          {libs.map((library) => (
            <LibraryTableRow key={library.id} library={library} {...actions} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

type LibraryTableRowProps = Omit<LibraryTableProps, 'libs'> & {
  library: Library
}

function LibraryTableRow({ library, ...actions }: LibraryTableRowProps) {
  return (
    <tr className="border-t border-gray-200">
      <td className="py-2 pr-3 font-medium text-ink-600">{library.name}</td>
      <td className="py-1.5 text-ink-100">
        <LibraryRootsCell library={library} {...actions} />
      </td>
      <td className="px-3 text-ink-100">{library.type}</td>
      <td className="py-2 text-right">
        <LibraryActionsCell library={library} {...actions} />
      </td>
    </tr>
  )
}

function LibraryRootsCell({ library, ...actions }: LibraryTableRowProps) {
  const roots = library.roots?.length ? library.roots : [fallbackLibraryRoot(library)]
  return (
    <div className="min-w-[520px] space-y-1">
      {roots.map((root) => (
        <ExistingRootEditor key={root.id || root.path} library={library} root={root} {...actions} />
      ))}
    </div>
  )
}

type RootEditorProps = Omit<LibraryTableRowProps, 'library'> & {
  library: Library
  root: LibraryRoot
}

function ExistingRootEditor({ library, root, ...actions }: RootEditorProps) {
  const draft = actions.editableRootDraft(library.id, root)
  return (
    <div className="grid items-center gap-1.5 rounded-lg border border-gray-200/80 bg-gray-50/60 p-1.5 xl:grid-cols-[minmax(92px,0.65fr)_minmax(240px,2fr)_auto_auto]">
      {root.id ? <EditableRootFields library={library} root={root} draft={draft} {...actions} /> : <ReadonlyRootFields root={root} />}
      <RootStatus enabled={draft.enabled ?? root.enabled} />
      <RootActionButtons library={library} root={root} draft={draft} {...actions} />
    </div>
  )
}

function ReadonlyRootFields({ root }: { root: LibraryRoot }) {
  return (
    <>
      <span className="truncate rounded-md bg-white/80 px-2.5 py-1.5 text-xs text-ink-600">{displayLibraryRootName(root.name, root.path)}</span>
      <span className="min-w-0 truncate rounded-md bg-white/80 px-2.5 py-1.5 text-xs text-ink-100" title={displayLibraryRootPath(root.path)}>
        {displayLibraryRootPath(root.path)}
      </span>
    </>
  )
}

function EditableRootFields({ library, root, draft, onEditableRootChange }: RootEditorProps & { draft: RootDraft }) {
  return (
    <>
      <input
        className="h-9 w-full rounded-lg border border-gray-200 bg-white/80 px-3 text-xs text-gray-900 outline-none transition focus:border-brand-500 focus:ring-2 focus:ring-brand-100/60"
        placeholder="路径名称"
        value={draft.name ?? ''}
        onChange={(e) => onEditableRootChange(library.id, root, { name: e.target.value })}
      />
      <input
        className="h-9 w-full rounded-lg border border-gray-200 bg-white/80 px-3 text-xs text-gray-900 outline-none transition focus:border-brand-500 focus:ring-2 focus:ring-brand-100/60"
        placeholder="真实路径"
        value={draft.path}
        onChange={(e) => onEditableRootChange(library.id, root, { path: e.target.value })}
      />
    </>
  )
}

function RootStatus({ enabled }: { enabled: boolean }) {
  return (
    <span
      className={`whitespace-nowrap rounded-md border px-2 py-1 text-xs ${
        enabled ? 'border-emerald-300/60 text-emerald-600' : 'border-gray-300 text-ink-50'
      }`}
    >
      {enabled ? '启用' : '禁用'}
    </span>
  )
}

function RootActionButtons({ library, root, draft, ...actions }: RootEditorProps & { draft: RootDraft }) {
  const enabled = draft.enabled ?? root.enabled
  return (
    <ActionMenu label="路径操作">
      {root.id && (
        <MenuButton
          icon={<Save size={14} />}
          label="保存"
          onClick={() => actions.onSaveRoot(library.id, root)}
        >
          保存
        </MenuButton>
      )}
      <MenuButton
        icon={<RefreshCw size={14} />}
        label="扫描"
        onClick={() => actions.onScanRoot(library.id, root)}
      >
        扫描
      </MenuButton>
      {root.id && (
        <MenuButton
          icon={enabled ? <PowerOff size={14} /> : <Power size={14} />}
          label={enabled ? '禁用' : '启用'}
          onClick={() => actions.onToggleRoot(library.id, root)}
        >
          {enabled ? '禁用' : '启用'}
        </MenuButton>
      )}
      {root.id && (
        <MenuButton
          danger
          icon={<Trash2 size={14} />}
          label="删除"
          onClick={() => actions.onRemoveRoot(library, root)}
        >
          删除
        </MenuButton>
      )}
    </ActionMenu>
  )
}

function LibraryActionsCell({ library, onScanLibrary, onRemoveLibrary }: LibraryTableRowProps) {
  return (
    <ActionMenu label="媒体库操作">
      <MenuButton icon={<RefreshCw size={14} />} label="扫描" onClick={() => onScanLibrary(library)}>
        扫描
      </MenuButton>
      <MenuButton danger icon={<Trash2 size={14} />} label="删除" onClick={() => onRemoveLibrary(library)}>
        删除
      </MenuButton>
    </ActionMenu>
  )
}

function ActionMenu({ label, children }: { label: string; children: ReactNode }) {
  return (
    <details className="group relative inline-flex justify-end">
      <summary
        className="flex h-8 w-8 cursor-pointer list-none items-center justify-center rounded-lg border border-gray-200 bg-white text-ink-50 transition hover:border-primary-400/50 hover:text-brand-500 [&::-webkit-details-marker]:hidden"
        title={label}
      >
        <MoreVertical size={16} />
      </summary>
      <div className="absolute right-0 top-9 z-30 min-w-28 rounded-lg border border-gray-200 bg-white p-1 shadow-lg">
        {children}
      </div>
    </details>
  )
}

function MenuButton({
  icon,
  label,
  danger,
  onClick,
  children,
}: {
  icon: ReactNode
  label: string
  danger?: boolean
  onClick: () => void
  children: ReactNode
}) {
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.currentTarget.closest('details')?.removeAttribute('open')
    onClick()
  }
  return (
    <button
      className={`flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs transition ${
        danger ? 'text-red-500 hover:bg-red-50' : 'text-ink-100 hover:bg-gray-50 hover:text-brand-500'
      }`}
      title={label}
      onClick={handleClick}
    >
      {icon}
      <span>{children}</span>
    </button>
  )
}
