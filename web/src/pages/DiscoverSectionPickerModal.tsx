import { useRef, useState } from 'react'
import { Check, Flame, GripVertical, Layers3, ListOrdered, Save, X } from 'lucide-react'

import type { DiscoverSection } from '../api/discover'
import { moveDiscoverSection, pointerReorderIndex } from './discoverSectionReorderModel'

export function DiscoverSectionPickerModal({
  sections,
  selected,
  saving,
  error,
  onToggle,
  onReorder,
  onClose,
  onSave,
}: {
  sections: DiscoverSection[]
  selected: string[]
  saving: boolean
  error?: string
  onToggle: (key: string) => void
  onReorder: (keys: string[]) => void
  onClose: () => void
  onSave: () => void
}) {
  const generalSections = sections.filter((section) => section.group !== 'adult')
  const adultSections = sections.filter((section) => section.group === 'adult')

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-labelledby="discover-section-picker-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !saving) onClose()
      }}
    >
      <div className="max-h-[88vh] w-full max-w-2xl overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="rounded-xl bg-primary-500/10 p-2 text-brand-500">
              <Layers3 size={18} />
            </span>
            <div className="min-w-0">
              <h2 id="discover-section-picker-title" className="text-lg font-semibold text-ink-600">
                选择与排序发现模块
              </h2>
              <p className="text-xs text-ink-50">已选择 {selected.length} 个模块，保存后按下方顺序展示</p>
            </div>
          </div>
          <button type="button" className="icon-button" title="关闭" onClick={onClose} disabled={saving}>
            <X size={17} />
          </button>
        </div>

        <div className="max-h-[64vh] space-y-6 overflow-y-auto p-5">
          {selected.length > 0 && (
            <DiscoverSelectedOrder
              sections={sections}
              selected={selected}
              disabled={saving}
              onReorder={onReorder}
            />
          )}
          <DiscoverSectionGroup
            title="影视与动漫"
            sections={generalSections}
            selected={selected}
            disabled={saving}
            onToggle={onToggle}
          />
          {adultSections.length > 0 && (
            <DiscoverSectionGroup
              title="成人专区"
              sections={adultSections}
              selected={selected}
              disabled={saving}
              adult
              onToggle={onToggle}
            />
          )}
          {error && (
            <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {error}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-gray-200 px-5 py-4">
          <p className="text-xs text-sand-500">可以清空全部模块，之后仍可在这里重新选择。</p>
          <div className="flex shrink-0 gap-2">
            <button
              type="button"
              className="rounded-lg border border-gray-300 px-3 py-2 text-sm text-ink-100 transition hover:border-gray-400"
              onClick={onClose}
              disabled={saving}
            >
              取消
            </button>
            <button type="button" className="neon-button inline-flex items-center gap-2" onClick={onSave} disabled={saving}>
              <Save size={15} />
              {saving ? '保存中' : '保存选择'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function DiscoverSelectedOrder({
  sections,
  selected,
  disabled,
  onReorder,
}: {
  sections: DiscoverSection[]
  selected: string[]
  disabled: boolean
  onReorder: (keys: string[]) => void
}) {
  const sectionMap = new Map(sections.map((section) => [section.key, section]))
  return (
    <section className="space-y-3">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-600">
        <ListOrdered size={16} />
        已选模块顺序
      </h3>
      <ol className="divide-y divide-gray-200 overflow-hidden rounded-xl border border-gray-200 bg-gray-50">
        {selected.map((key, index) => {
          const section = sectionMap.get(key)
          if (!section) return null
          return (
            <DiscoverSelectedOrderItem
              key={key}
              section={section}
              index={index}
              disabled={disabled}
              selected={selected}
              onReorder={onReorder}
            />
          )
        })}
      </ol>
    </section>
  )
}

function DiscoverSelectedOrderItem({
  section,
  index,
  disabled,
  selected,
  onReorder,
}: {
  section: DiscoverSection
  index: number
  disabled: boolean
  selected: string[]
  onReorder: (keys: string[]) => void
}) {
  const pointerIdRef = useRef<number | null>(null)
  const [dragging, setDragging] = useState(false)

  const finishDrag = (element: HTMLLIElement, pointerId: number) => {
    if (pointerIdRef.current !== pointerId) return
    if (element.hasPointerCapture(pointerId)) element.releasePointerCapture(pointerId)
    pointerIdRef.current = null
    setDragging(false)
  }

  return (
    <li
      data-discover-section-key={section.key}
      onPointerDown={(event) => {
        if (disabled || !event.isPrimary || event.button !== 0) return
        event.preventDefault()
        pointerIdRef.current = event.pointerId
        event.currentTarget.setPointerCapture(event.pointerId)
        setDragging(true)
      }}
      onPointerMove={(event) => {
        if (pointerIdRef.current !== event.pointerId) return
        event.preventDefault()
        const list = event.currentTarget.parentElement
        if (!list) return
        const items = Array.from(list.querySelectorAll<HTMLLIElement>('[data-discover-section-key]')).filter(
          (item) => item !== event.currentTarget,
        )
        const targetIndex = pointerReorderIndex(
          event.clientY,
          items.map((item) => {
            const rect = item.getBoundingClientRect()
            return rect.top + rect.height / 2
          }),
        )
        const next = moveDiscoverSection(selected, section.key, targetIndex)
        if (next !== selected) onReorder(next)
      }}
      onPointerUp={(event) => {
        finishDrag(event.currentTarget, event.pointerId)
      }}
      onPointerCancel={(event) => {
        finishDrag(event.currentTarget, event.pointerId)
      }}
      onLostPointerCapture={(event) => {
        if (pointerIdRef.current !== event.pointerId) return
        pointerIdRef.current = null
        setDragging(false)
      }}
      className={
        'flex touch-none select-none items-center gap-3 px-3 py-2.5 transition-[transform,box-shadow,background-color] ' +
        (dragging
          ? 'relative z-10 scale-[1.01] cursor-grabbing bg-white shadow-[0_12px_28px_rgba(15,23,42,0.14)]'
          : 'cursor-grab bg-gray-50')
      }
    >
      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-white text-xs font-semibold text-sand-500 shadow-sm">
        {index + 1}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink-600">{section.label}</span>
      <button
        type="button"
        className="touch-none rounded-md border border-gray-200 bg-white p-1.5 text-sand-500 transition hover:border-primary-200 hover:text-brand-500 active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-35"
        title="上下拖动调整顺序"
        aria-label={`拖动排序 ${section.label}`}
        disabled={disabled}
      >
        <GripVertical size={16} className="cursor-grab active:cursor-grabbing" />
      </button>
    </li>
  )
}

function DiscoverSectionGroup({
  title,
  sections,
  selected,
  disabled,
  adult = false,
  onToggle,
}: {
  title: string
  sections: DiscoverSection[]
  selected: string[]
  disabled: boolean
  adult?: boolean
  onToggle: (key: string) => void
}) {
  return (
    <section className="space-y-3">
      <h3 className={`flex items-center gap-2 text-sm font-semibold ${adult ? 'text-rose-700' : 'text-ink-600'}`}>
        {adult ? <Flame size={16} /> : <Layers3 size={16} />}
        {title}
      </h3>
      <div className="grid gap-2 sm:grid-cols-2">
        {sections.map((section) => {
          const active = selected.includes(section.key)
          return (
            <button
              key={section.key}
              type="button"
              aria-pressed={active}
              disabled={disabled}
              onClick={() => onToggle(section.key)}
              className={
                'flex items-center justify-between gap-3 rounded-xl border px-3 py-3 text-left text-sm font-medium transition ' +
                (active
                  ? adult
                    ? 'border-rose-300 bg-rose-50 text-rose-700'
                    : 'border-primary-300 bg-primary-500/10 text-brand-500'
                  : 'border-gray-200 bg-white text-ink-100 hover:border-primary-200 hover:bg-gray-50')
              }
            >
              <span>{section.label}</span>
              <span
                className={
                  'flex h-5 w-5 shrink-0 items-center justify-center rounded-md border ' +
                  (active
                    ? adult
                      ? 'border-rose-500 bg-rose-500 text-white'
                      : 'border-primary-500 bg-primary-500 text-white'
                    : 'border-gray-300 bg-white text-transparent')
                }
              >
                <Check size={13} strokeWidth={3} />
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}
