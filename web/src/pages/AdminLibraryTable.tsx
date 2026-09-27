import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Image, ImageOff, MoreVertical, Play, Plus, Power, PowerOff, RefreshCw, Save, Square, Trash2 } from 'lucide-react'

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
  onToggleLibrary: (library: Library) => void
  onScanLibrary: (library: Library) => void
  onTitleModeChange: (library: Library, titleMode: 'smart' | 'filename') => void
  onGenerateArtworkChange: (library: Library, enabled: boolean) => void
  onRunGeneratedArtwork: (library: Library) => void
  onCancelGeneratedArtwork: (library: Library) => void
  onRemoveLibrary: (library: Library) => void
  onAddLibraryRoot: (library: Library) => void
  onEditLibraryCover: (library: Library) => void
}

export function AdminLibraryTable({ libs, ...actions }: LibraryTableProps) {
  return (
    <div className="glass-panel overflow-x-auto !p-3">
      <table className="w-full min-w-[900px] text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="w-28 py-2">名称</th>
            <th className="w-28">媒体库状态</th>
            <th>路径</th>
            <th className="w-20">类型</th>
            <th className="w-36">标题模式</th>
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
      <td className="py-2 pr-3 font-medium text-ink-600">
        <div className="flex items-center gap-2">
          {library.cover_url && <img src={library.cover_url} alt="" className="h-10 w-8 rounded object-cover" />}
          <span>{library.name}</span>
        </div>
      </td>
      <td className="pr-3">
        <StatusBadge enabled={library.enabled} enabledLabel="媒体库启用" disabledLabel="媒体库停用" />
      </td>
      <td className="py-1.5 text-ink-100">
        <LibraryRootsCell library={library} {...actions} />
      </td>
      <td className="px-3 text-ink-100">{library.type}</td>
      <td className="px-3">
        <select
          className="h-9 w-full rounded-lg border border-gray-200 bg-white px-2 text-xs text-ink-100"
          value={library.title_mode || 'smart'}
          onChange={(event) => actions.onTitleModeChange(library, event.target.value as 'smart' | 'filename')}
        >
          <option value="smart">智能识别</option>
          <option value="filename">原始文件名</option>
        </select>
      </td>
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
      <StatusBadge enabled={draft.enabled ?? root.enabled} enabledLabel="路径启用" disabledLabel="路径禁用" />
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

function StatusBadge({ enabled, enabledLabel, disabledLabel }: { enabled: boolean; enabledLabel: string; disabledLabel: string }) {
  return (
    <span
      className={`whitespace-nowrap rounded-md border px-2 py-1 text-xs ${
        enabled ? 'border-emerald-300/60 text-emerald-600' : 'border-gray-300 text-ink-50'
      }`}
    >
      {enabled ? enabledLabel : disabledLabel}
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

function LibraryActionsCell({
  library,
  onToggleLibrary,
  onScanLibrary,
  onGenerateArtworkChange,
  onRunGeneratedArtwork,
  onCancelGeneratedArtwork,
  onRemoveLibrary,
  onAddLibraryRoot,
  onEditLibraryCover,
}: LibraryTableRowProps) {
  return (
    <ActionMenu label="媒体库操作">
      <MenuButton
        icon={library.enabled ? <PowerOff size={14} /> : <Power size={14} />}
        label={library.enabled ? '停用媒体库' : '启用媒体库'}
        onClick={() => onToggleLibrary(library)}
      >
        {library.enabled ? '停用媒体库' : '启用媒体库'}
      </MenuButton>
      <MenuButton icon={<RefreshCw size={14} />} label="扫描" onClick={() => onScanLibrary(library)}>
        扫描
      </MenuButton>
      <MenuButton icon={<Plus size={14} />} label="添加来源" onClick={() => onAddLibraryRoot(library)}>
        添加来源
      </MenuButton>
      <MenuButton icon={<Image size={14} />} label="自定义封面" onClick={() => onEditLibraryCover(library)}>
        自定义封面
      </MenuButton>
      <MenuButton
        icon={library.generate_artwork ? <ImageOff size={14} /> : <Image size={14} />}
        label={library.generate_artwork ? '关闭缺图预览' : '开启缺图预览'}
        onClick={() => onGenerateArtworkChange(library, !library.generate_artwork)}
      >
        {library.generate_artwork ? '关闭缺图预览' : '开启缺图预览'}
      </MenuButton>
      {library.generate_artwork && (
        <MenuButton icon={<Play size={14} />} label="生成缺失预览" onClick={() => onRunGeneratedArtwork(library)}>
          生成缺失预览
        </MenuButton>
      )}
      {library.generate_artwork && (
        <MenuButton icon={<Square size={14} />} label="停止生成" onClick={() => onCancelGeneratedArtwork(library)}>
          停止生成
        </MenuButton>
      )}
      <MenuButton danger icon={<Trash2 size={14} />} label="删除" onClick={() => onRemoveLibrary(library)}>
        删除
      </MenuButton>
    </ActionMenu>
  )
}

function ActionMenu({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState({ left: 0, top: 0 })
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const updatePosition = useCallback(() => {
    const trigger = triggerRef.current
    if (!trigger) return
    const triggerRect = trigger.getBoundingClientRect()
    const menuRect = menuRef.current?.getBoundingClientRect()
    const menuWidth = menuRect?.width || 176
    const menuHeight = menuRect?.height || 0
    const viewportMargin = 8
    const gap = 4
    const maxLeft = Math.max(viewportMargin, window.innerWidth - menuWidth - viewportMargin)
    const left = Math.min(maxLeft, Math.max(viewportMargin, triggerRect.right - menuWidth))
    const belowSpace = window.innerHeight - triggerRect.bottom - viewportMargin
    const aboveSpace = triggerRect.top - viewportMargin
    let top = triggerRect.bottom + gap
    if (menuHeight > belowSpace && aboveSpace > belowSpace) {
      top = triggerRect.top - menuHeight - gap
    }
    if (menuHeight > 0) {
      top = Math.min(
        Math.max(viewportMargin, top),
        Math.max(viewportMargin, window.innerHeight - menuHeight - viewportMargin),
      )
    }
    setPosition({ left, top })
  }, [])

  useLayoutEffect(() => {
    if (open) updatePosition()
  }, [open, updatePosition])

  useEffect(() => {
    if (!open) return
    const closeOnOutsideClick = (event: globalThis.MouseEvent) => {
      const target = event.target as Node
      if (triggerRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('mousedown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      document.removeEventListener('mousedown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open, updatePosition])

  return (
    <span className="inline-flex justify-end">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 bg-white text-ink-50 transition hover:border-primary-400/50 hover:text-brand-500"
        title={label}
        onClick={() => setOpen((current) => !current)}
      >
        <MoreVertical size={16} />
      </button>
      {open && createPortal(
        <div
          ref={menuRef}
          role="menu"
          aria-label={label}
          className="fixed z-[90] min-w-36 whitespace-nowrap rounded-lg border border-gray-200 bg-white p-1 shadow-xl"
          style={{ left: position.left, top: position.top }}
          onClick={() => setOpen(false)}
        >
          {children}
        </div>,
        document.body,
      )}
    </span>
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
  const handleClick = () => {
    onClick()
  }
  return (
    <button
      type="button"
      role="menuitem"
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
