import { useEffect, useRef, useState } from 'react'
import { motion, Reorder, useDragControls, useReducedMotion } from 'framer-motion'
import { ArrowUpToLine, GripVertical } from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'

const settingKey = 'emby.library_display'
type DisplayEntry = { id: string; hidden: boolean }
type DisplayRow = DisplayEntry & { name: string }

export function EmbyLibraryDisplayDialog({ onClose }: { onClose: () => void }) {
  const [rows, setRows] = useState<DisplayRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [saving, setSaving] = useState(false)
  const submitting = useRef(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([libraryAPI.list({ includeHidden: true }), adminAPI.listSettings()]).then(([libs, settings]) => {
      if (cancelled) return
      const value = settings.find((setting) => setting.key === settingKey)?.value
      const entries: DisplayEntry[] = value ? JSON.parse(value) : []
      if (!Array.isArray(entries) || entries.some((entry) => !entry || typeof entry.id !== 'string' || typeof entry.hidden !== 'boolean')) throw new Error('invalid display settings')
      const remaining = new Map(libs.map((lib) => [lib.id, lib]))
      const ordered: DisplayRow[] = []
      for (const entry of entries) {
        const lib = remaining.get(entry.id)
        if (lib) ordered.push({ ...entry, name: lib.name })
        remaining.delete(entry.id)
      }
      setRows([...ordered, ...Array.from(remaining.values(), (lib) => ({ id: lib.id, name: lib.name, hidden: false }))])
    }).catch(() => { if (!cancelled) setError(true) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [attempt])

  const move = (index: number, offset: number) => {
    if (saving || index + offset < 0 || index + offset >= rows.length) return
    const next = [...rows]
    next.splice(index + offset, 0, next.splice(index, 1)[0])
    setRows(next)
  }

  const save = async () => {
    if (submitting.current || loading || error) return
    submitting.current = true
    setSaving(true)
    try {
      await adminAPI.updateSetting(settingKey, JSON.stringify(rows.map(({ id, hidden }) => ({ id, hidden }))))
      toast.success('Emby 媒体库展示已保存')
      onClose()
    } catch {
      toast.error('保存失败，请重试；若媒体库已变更，请重新打开弹窗')
    } finally {
      submitting.current = false
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"><div role="dialog" aria-modal="true" aria-label="Emby 媒体库展示" className="flex max-h-[85dvh] w-full max-w-xl flex-col rounded-2xl bg-[var(--app-bg)] shadow-xl">
      <form className="flex min-h-0 flex-1 flex-col" onSubmit={(event) => { event.preventDefault(); void save() }} onKeyDown={(event) => {
        if (event.key === 'Escape' && !saving) onClose()
        if (event.key !== 'Tab') return
        const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)'))
        const first = controls[0], last = controls[controls.length - 1]
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
      }}>
        <div className="border-b border-ink-100/10 p-4">
          <div>
            <h3 className="font-display text-lg font-bold text-ink-600">Emby 媒体库展示</h3>
            <p className="mt-1 text-sm text-ink-50">拖拽调整顺序，统一设置所有用户的媒体库入口。</p>
            <p className="mt-1 text-xs text-ink-50">隐藏仅影响 Emby 入口，不改变搜索、播放和用户权限。</p>
          </div>
        </div>
        <motion.div layoutScroll className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {loading ? <p role="status">加载中…</p> : error ? <div role="alert" className="space-y-3">
            <p>加载失败，请重试。</p>
            <button type="button" className="btn-outline" onClick={() => { setError(false); setLoading(true); setAttempt(attempt + 1) }}>重试</button>
          </div> : rows.length === 0 ? <p className="text-sm text-ink-50">暂无媒体库</p> : (
            <Reorder.Group as="ol" axis="y" values={rows} onReorder={(next) => { if (!saving) setRows(next) }} className="space-y-2">
              {rows.map((row, index) => <DisplayItem key={row.id} row={row} index={index} saving={saving} move={move}
                toggle={() => setRows(rows.map((entry) => entry.id === row.id ? { ...entry, hidden: !entry.hidden } : entry))} />)}
            </Reorder.Group>
          )}
        </motion.div>
        <div className="flex justify-end gap-2 border-t border-ink-100/10 p-4">
          <button autoFocus type="button" className="btn-outline" disabled={saving} onClick={onClose}>取消</button>
          <button type="submit" className="btn-primary" disabled={loading || error || saving}>{saving ? '保存中…' : '保存'}</button>
        </div>
      </form>
    </div></div>
  )
}

function DisplayItem({ row, index, saving, move, toggle }: {
  row: DisplayRow; index: number; saving: boolean
  move: (index: number, offset: number) => void; toggle: () => void
}) {
  const controls = useDragControls()
  const reduced = useReducedMotion()
  return (
    <Reorder.Item value={row} dragListener={false} dragControls={controls} layout="position" transition={reduced ? { duration: 0 } : undefined}
      className="relative flex items-center gap-1 rounded-xl border border-ink-100/10 bg-[var(--app-bg)] px-2 py-1">
      <button type="button" disabled={saving} className="flex min-h-11 w-7 shrink-0 touch-none items-center justify-center rounded-lg text-ink-50 active:cursor-grabbing"
        aria-label={`拖动${row.name}排序，也可使用上下方向键`} onPointerDown={(event) => { if (!saving) controls.start(event) }}
        onKeyDown={(event) => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); move(index, event.key === 'ArrowUp' ? -1 : 1) } }}><GripVertical size={20} /></button>
      <div className="w-7 shrink-0">
        {index > 0 && <button type="button" className="icon-btn !h-11 !w-7" disabled={saving} title="置顶" aria-label={`置顶${row.name}`} onClick={() => move(index, -index)}><ArrowUpToLine size={16} /></button>}
      </div>
      <span className="min-w-0 flex-1 break-words text-sm font-semibold">{row.name}</span>
      <label className="flex min-h-11 shrink-0 cursor-pointer items-center gap-2 px-1 text-sm">
        <input type="checkbox" role="switch" className="h-4 w-4 accent-brand-500" checked={!row.hidden} disabled={saving} onChange={toggle} aria-label={`显示${row.name}`} />
        {row.hidden ? '隐藏' : '显示'}
      </label>
    </Reorder.Item>
  )
}
