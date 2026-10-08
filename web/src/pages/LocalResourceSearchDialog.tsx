import { useEffect, useRef } from 'react'
import { X } from 'lucide-react'
import { useResourceImportCapability } from '../hooks/useResourceImportCapability'
import { PTResourceSearchPanel } from './PTResourceSearchPanel'
import type { PTResourceMetadata } from './ptResourceModel'

export type LocalResourceSearch = { query: string; metadata?: PTResourceMetadata; onCloud: () => void }

export function LocalResourceSearchDialog({ search, capability, onClose }: {
  search: LocalResourceSearch
  capability: ReturnType<typeof useResourceImportCapability>
  onClose: () => void
}) {
  const dialogRef = useRef<HTMLElement>(null)
  const onCloseRef = useRef(onClose)
  useEffect(() => { onCloseRef.current = onClose }, [onClose])
  useEffect(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const focusable = () => Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex="0"]') ?? [])
    ;(focusable()[0] ?? dialogRef.current)?.focus()
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onCloseRef.current(); return }
      if (event.key !== 'Tab') return
      const elements = focusable()
      const first = elements[0]
      const last = elements[elements.length - 1]
      if (!first) { event.preventDefault(); dialogRef.current?.focus(); return }
      if (event.shiftKey && (document.activeElement === first || !dialogRef.current?.contains(document.activeElement))) { event.preventDefault(); last.focus() }
      else if (!event.shiftKey && (document.activeElement === last || !dialogRef.current?.contains(document.activeElement))) { event.preventDefault(); first.focus() }
    }
    window.addEventListener('keydown', close)
    return () => { window.removeEventListener('keydown', close); if (previousFocus?.isConnected) previousFocus.focus() }
  }, [])
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
    <section ref={dialogRef} tabIndex={-1} role="dialog" aria-modal="true" aria-label="搜索资源与补集" className="max-h-[90dvh] w-full max-w-3xl overflow-y-auto rounded-2xl bg-[var(--app-bg)] p-5 shadow-xl">
      <header className="mb-4 flex items-center justify-between gap-4"><h2 className="text-lg font-semibold">搜索 PT 资源 / 补集</h2><button type="button" aria-label="关闭资源搜索" className="btn-outline" onClick={onClose}><X size={18} /></button></header>
      <PTResourceSearchPanel initialQuery={search.query} metadata={search.metadata} />
      <div className="mt-5 border-t border-gray-200 pt-4 text-sm text-sand-500">
        {capability.state === 'enabled' && <button type="button" className="btn-outline" onClick={() => { onClose(); search.onCloud() }}>使用网盘搜索入库（可选）</button>}
        {capability.state === 'loading' && <p>正在检查可选网盘入库功能…</p>}
        {capability.state === 'disabled' && <p>网盘入库未配置，PT 搜索和下载可正常使用。</p>}
        {capability.state === 'error' && <div role="alert">无法检查网盘入库状态。<button type="button" className="ml-2 text-brand-500" onClick={capability.retry}>重试</button></div>}
      </div>
    </section>
  </div>
}
