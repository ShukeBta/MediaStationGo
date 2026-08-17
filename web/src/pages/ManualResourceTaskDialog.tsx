import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link2, LoaderCircle, X } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  resourceImportsAPI,
  type ResourceImportTask,
  type ResourceSearchResponse,
} from '../api/resourceImports'
import { libraryAPI } from '../api/library'
import { confirmAction } from '../components/confirmAction'
import type { Library } from '../types'
import {
  manualResourcePreviewSelection,
  manualResourceTypeLabel,
} from './manualResourceTaskModel'
import { resourceImportDuplicateConflict, resourceImportError } from './resourceImportModel'

export function ManualResourceTaskDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: (task: ResourceImportTask) => void
}) {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [libraryID, setLibraryID] = useState('')
  const [input, setInput] = useState('')
  const [preview, setPreview] = useState<ResourceSearchResponse | null>(null)
  const [loadingLibraries, setLoadingLibraries] = useState(true)
  const [parsing, setParsing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  const selectedLibrary = useMemo(
    () => libraries.find((library) => library.id === libraryID) ?? null,
    [libraries, libraryID],
  )

  useEffect(() => {
    let cancelled = false
    libraryAPI.list().then((items) => {
      if (cancelled) return
      const enabled = items.filter((library) => library.enabled)
      setLibraries(enabled)
      setLibraryID((current) => current || enabled[0]?.id || '')
      setLoadingLibraries(false)
    }).catch((requestError) => {
      if (cancelled) return
      setError(resourceImportError(requestError, '媒体库加载失败'))
      setLoadingLibraries(false)
    })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !parsing && !creating) onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [creating, onClose, parsing])

  const updateInput = (value: string) => {
    setInput(value)
    setPreview(null)
    setError('')
  }

  const updateLibrary = (value: string) => {
    setLibraryID(value)
    setPreview(null)
    setError('')
  }

  const parseInput = async (event: FormEvent) => {
    event.preventDefault()
    if (!libraryID || !input.trim() || parsing) return
    setParsing(true)
    setError('')
    setPreview(null)
    try {
      const next = await resourceImportsAPI.previewManual(libraryID, input.trim())
      manualResourcePreviewSelection(next)
      setPreview(next)
    } catch (requestError) {
      setError(resourceImportError(requestError, '链接解析失败'))
    } finally {
      setParsing(false)
    }
  }

  const createTask = async (forceDuplicate = false) => {
    if (!preview || creating) return
    const { candidate, root } = manualResourcePreviewSelection(preview)
    setCreating(true)
    setError('')
    try {
      const task = await resourceImportsAPI.create(libraryID, {
        search_session_id: preview.session_id,
        candidate_index: candidate.index,
        root_id: root.id,
        force_duplicate: forceDuplicate || undefined,
      })
      toast.success('任务已创建')
      onCreated(task)
      onClose()
    } catch (requestError) {
      const conflict = resourceImportDuplicateConflict(requestError)
      if (!forceDuplicate && conflict?.can_force) {
        setCreating(false)
        const confirmed = await confirmAction({
          title: '确认重复入库',
          message: conflict.message,
          confirmText: '仍然创建',
          cancelText: '取消',
          danger: false,
        })
        if (confirmed) await createTask(true)
        return
      }
      setError(conflict?.message || resourceImportError(requestError, '任务创建失败'))
    } finally {
      setCreating(false)
    }
  }

  const selection = preview ? manualResourcePreviewSelection(preview) : null
  const busy = parsing || creating

  return (
    <div
      className="fixed inset-0 z-[80] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      onClick={() => !busy && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="新建任务"
        className="w-full max-w-xl overflow-hidden rounded-lg border border-white/70 bg-[var(--app-panel)] shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="flex h-16 items-center gap-3 border-b border-gray-200 px-5">
          <Link2 size={20} className="text-brand-500" />
          <h2 className="min-w-0 flex-1 font-display text-lg font-bold text-ink-600">新建任务</h2>
          <button
            type="button"
            className="rounded-lg p-2 text-sand-500 hover:bg-gray-100 hover:text-ink-600"
            title="关闭"
            aria-label="关闭新建任务"
            disabled={busy}
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </header>

        <form className="space-y-4 px-5 py-5" onSubmit={parseInput}>
          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-ink-100">磁链 / 115 分享链接</span>
            <textarea
              required
              rows={4}
              className="input-base resize-y break-all font-mono text-sm"
              placeholder="magnet:?xt=... 或 https://115.com/s/..."
              value={input}
              onChange={(event) => updateInput(event.target.value)}
            />
          </label>

          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-ink-100">媒体库</span>
            <select
              required
              className="input-base"
              value={libraryID}
              disabled={loadingLibraries}
              onChange={(event) => updateLibrary(event.target.value)}
            >
              {loadingLibraries && <option value="">加载中…</option>}
              {!loadingLibraries && libraries.length === 0 && <option value="">没有可用媒体库</option>}
              {libraries.map((library) => (
                <option key={library.id} value={library.id}>{library.name}</option>
              ))}
            </select>
          </label>

          {error && <p className="break-words text-sm text-red-500">{error}</p>}

          {selection && (
            <div className="border-y border-gray-200 py-4">
              <div className="flex flex-wrap items-center gap-2 text-xs text-sand-500">
                <span>{manualResourceTypeLabel(selection.candidate)}</span>
                <span>·</span>
                <span>{selectedLibrary?.name}</span>
              </div>
              <p className="mt-1 break-words text-sm font-semibold text-ink-600">
                {selection.candidate.title}
              </p>
            </div>
          )}

          <div className="flex justify-end gap-2 border-t border-gray-100 pt-4">
            <button type="button" className="btn-outline px-4 py-2" disabled={busy} onClick={onClose}>
              取消
            </button>
            {preview ? (
              <button type="button" className="btn-primary px-4 py-2" disabled={busy} onClick={() => void createTask()}>
                {creating && <LoaderCircle size={16} className="animate-spin" />}
                创建任务
              </button>
            ) : (
              <button type="submit" className="btn-primary px-4 py-2" disabled={busy || !libraryID || !input.trim()}>
                {parsing && <LoaderCircle size={16} className="animate-spin" />}
                解析
              </button>
            )}
          </div>
        </form>
      </div>
    </div>
  )
}
