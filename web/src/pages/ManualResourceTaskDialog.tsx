import { FormEvent, useEffect, useState } from 'react'
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
import { manualResourcePreviewSelection } from './manualResourceTaskModel'
import { resourceImportDuplicateConflict, resourceImportError } from './resourceImportModel'

export function ManualResourceTaskDialog({
  onClose,
  onCreated,
  fixedLibraryID,
  fixedLibraryName,
  fixedRootID,
  upgradeMediaID,
  upgradeScope,
  canRemoveOldVersion = false,
  replenishMediaID,
}: {
  onClose: () => void
  onCreated: (task: ResourceImportTask) => void
  fixedLibraryID?: string
  fixedLibraryName?: string
  fixedRootID?: string
  upgradeMediaID?: string
  upgradeScope?: 'media' | 'work'
  canRemoveOldVersion?: boolean
  replenishMediaID?: string
}) {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [libraryID, setLibraryID] = useState('')
  const [input, setInput] = useState('')
  const [title, setTitle] = useState('')
  const [loadingLibraries, setLoadingLibraries] = useState(true)
  const [parsing, setParsing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [keepOldVersion, setKeepOldVersion] = useState(true)
  const upgrading = Boolean(upgradeMediaID?.trim())
  const replenishing = Boolean(replenishMediaID?.trim())
  const dialogTitle = replenishing ? '补集' : upgrading ? '直接添加片源' : '新建任务'

  useEffect(() => {
    if (fixedLibraryID?.trim()) {
      setLibraryID(fixedLibraryID.trim())
      setLoadingLibraries(false)
      return undefined
    }
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
  }, [fixedLibraryID])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !parsing && !creating) onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [creating, onClose, parsing])

  const updateInput = (value: string) => {
    setInput(value)
    setError('')
  }

  const updateTitle = (value: string) => {
    setTitle(value)
    setError('')
  }

  const updateLibrary = (value: string) => {
    setLibraryID(value)
    setError('')
  }

  const parseInput = async (event: FormEvent) => {
    event.preventDefault()
    if (!input.trim() || parsing || creating) return
    if (!replenishing && (!libraryID || !title.trim())) return
    setParsing(true)
    setError('')
    try {
      if (replenishing) {
        const task = await resourceImportsAPI.replenishEpisodes(replenishMediaID?.trim() || '', input.trim())
        toast.success('补集任务已创建')
        onCreated(task)
        onClose()
        return
      }
      const next = await resourceImportsAPI.previewManual(libraryID, title.trim(), input.trim(), fixedRootID)
      manualResourcePreviewSelection(next)
      await createTask(next)
    } catch (requestError) {
      setError(resourceImportError(requestError, '任务准备失败'))
    } finally {
      setParsing(false)
    }
  }

  const createTask = async (preview: ResourceSearchResponse, forceDuplicate = false) => {
    if (creating) return
    const { candidate, root } = manualResourcePreviewSelection(preview)
    setCreating(true)
    setError('')
    try {
      const task = await resourceImportsAPI.create(libraryID, {
        search_session_id: preview.session_id,
        candidate_index: candidate.index,
        root_id: root.id,
        force_duplicate: forceDuplicate || undefined,
        upgrade_media_id: upgrading ? upgradeMediaID?.trim() || undefined : undefined,
        upgrade_scope: upgrading ? upgradeScope : undefined,
        keep_old_version: upgrading ? keepOldVersion : undefined,
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
        if (confirmed) await createTask(preview, true)
        return
      }
      setError(conflict?.message || resourceImportError(requestError, '任务创建失败'))
    } finally {
      setCreating(false)
    }
  }

  const busy = parsing || creating

  return (
    <div
      className="fixed inset-0 z-[90] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      onClick={() => !busy && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={dialogTitle}
        className="w-full max-w-xl overflow-hidden rounded-lg border border-white/70 bg-[var(--app-panel)] shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="flex h-16 items-center gap-3 border-b border-gray-200 px-5">
          <Link2 size={20} className="text-brand-500" />
          <h2 className="min-w-0 flex-1 font-display text-lg font-bold text-ink-600">{dialogTitle}</h2>
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
          {!replenishing && <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-ink-100">任务名称</span>
            <input
              required
              maxLength={200}
              className="input-base"
              placeholder="例如：凡人修仙传 第五季"
              value={title}
              onChange={(event) => updateTitle(event.target.value)}
            />
          </label>}
          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-ink-100">115 分享链接 / 磁链 / ED2K</span>
            <textarea
              required
              rows={4}
              className="input-base resize-y break-all font-mono text-sm"
              placeholder="https://115.com/s/...、magnet:?xt=... 或 ed2k://..."
              value={input}
              onChange={(event) => updateInput(event.target.value)}
            />
          </label>

          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-ink-100">媒体库</span>
            {fixedLibraryID ? (
              <p className="input-base truncate bg-gray-50 text-ink-100">{fixedLibraryName || '当前媒体库'}</p>
            ) : (
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
            )}
          </label>

          {error && <p className="break-words text-sm text-red-500">{error}</p>}

          {upgrading && canRemoveOldVersion && (
            <label className="flex items-center gap-2 text-xs font-semibold text-ink-100">
              <input
                type="checkbox"
                checked={keepOldVersion}
                onChange={(event) => setKeepOldVersion(event.target.checked)}
              />
              <span>保留旧版本</span>
            </label>
          )}

          <div className="flex justify-end gap-2 border-t border-gray-100 pt-4">
            <button type="button" className="btn-outline px-4 py-2" disabled={busy} onClick={onClose}>
              取消
            </button>
            <button type="submit" className="btn-primary px-4 py-2" disabled={busy || !input.trim() || (!replenishing && (!libraryID || !title.trim()))}>
              {busy && <LoaderCircle size={16} className="animate-spin" />}
              {replenishing ? '补集' : '创建任务'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
