import { useEffect, useMemo, useState } from 'react'
import { ArrowRightLeft, LoaderCircle, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { mediaAPI, type MediaMigrationCandidate, type MediaMigrationPreview } from '../api/library'
import { confirmAction } from './confirmAction'
import type { Media } from '../types'

interface MoveMediaLibraryDialogProps {
  open: boolean
  media: Media | null
  onClose: () => void
  onMoved?: () => void | Promise<void>
}

const CATEGORY_LABELS: Record<string, string> = {
  movie: '电影媒体库',
  tv: '电视剧媒体库',
  anime: '动漫媒体库',
  adult: '成人媒体库',
  other: '其他媒体库',
}

export function MoveMediaLibraryDialog({ open, media, onClose, onMoved }: MoveMediaLibraryDialogProps) {
  const [candidate, setCandidate] = useState<MediaMigrationCandidate | null>(null)
  const [targetCategories, setTargetCategories] = useState<string[]>([])
  const [targetCategory, setTargetCategory] = useState('')
  const [preview, setPreview] = useState<MediaMigrationPreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [validating, setValidating] = useState(false)
  const [applying, setApplying] = useState(false)

  useEffect(() => {
    if (!open || !media) return
    let alive = true
    setCandidate(null)
    setTargetCategories([])
    setTargetCategory('')
    setPreview(null)
    setLoading(true)
    mediaAPI.getMigration(media.id)
      .then((result) => {
        if (!alive) return
        setCandidate(result.candidate)
        setTargetCategories(result.target_categories ?? [])
        setTargetCategory(result.target_categories?.[0] ?? '')
      })
      .catch((error: unknown) => {
        if (alive) toast.error(apiErrorMessage(error, '加载移动媒体库入口失败'))
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [open, media])

  const targetLabel = useMemo(() => CATEGORY_LABELS[targetCategory] ?? targetCategory, [targetCategory])
  const canValidate = Boolean(candidate && targetCategory && !loading && !validating && !applying)

  if (!open || !media) return null

  const validate = async () => {
    if (!canValidate) return
    setValidating(true)
    setPreview(null)
    try {
      const result = await mediaAPI.validateMigration(media.id, targetCategory)
      setPreview(result)
      toast.success('校验通过，请确认移动')
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, '移动媒体库校验失败'))
    } finally {
      setValidating(false)
    }
  }

  const apply = async () => {
    if (!preview || applying) return
    setApplying(true)
    const confirmed = await confirmAction({
      title: '确认移动媒体库',
      message: `将「${media.title}」从「${candidate?.library_name || '当前媒体库'}」移动到「${targetLabel}」？会同步更新云盘路径、媒体库归属和去重索引。`,
      confirmText: '确认移动',
    })
    if (!confirmed) {
      setApplying(false)
      return
    }
    try {
      await mediaAPI.applyMigration(media.id, targetCategory)
      toast.success('媒体库移动完成')
      await onMoved?.()
      onClose()
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, '移动媒体库失败'))
    } finally {
      setApplying(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-8 backdrop-blur-sm">
      <div className="flex max-h-[88vh] w-full max-w-xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="font-display text-xl font-bold text-gray-900">移动媒体库</h2>
            <p className="mt-1 text-xs text-gray-500">管理员可将作品及其片源整体移动到另一媒体库。</p>
          </div>
          <button type="button" onClick={onClose} className="btn-ghost h-9 w-9 p-0" aria-label="关闭">
            <X size={16} />
          </button>
        </div>

        <div className="space-y-4 overflow-y-auto p-5">
          {loading && (
            <div className="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-3 text-sm text-gray-600">
              <LoaderCircle size={16} className="animate-spin" /> 正在读取作品移动信息…
            </div>
          )}
          {candidate && (
            <div className="space-y-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-3 text-sm text-gray-700">
              <div><span className="font-semibold text-gray-500">当前媒体库：</span>{candidate.library_name}</div>
              <div><span className="font-semibold text-gray-500">待移动内容：</span>{candidate.media_count} 个媒体项</div>
              <div><span className="font-semibold text-gray-500">当前路径：</span><span className="break-all">{candidate.source_openlist_path}</span></div>
            </div>
          )}
          {candidate && targetCategories.length > 0 && (
            <label className="block">
              <span className="mb-1 block text-xs font-bold text-gray-500">目标媒体库</span>
              <select
                value={targetCategory}
                onChange={(event) => {
                  setTargetCategory(event.target.value)
                  setPreview(null)
                }}
                className="h-11 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
              >
                {targetCategories.map((category) => (
                  <option key={category} value={category}>{CATEGORY_LABELS[category] ?? category}</option>
                ))}
              </select>
            </label>
          )}
          {candidate && targetCategories.length === 0 && (
            <p className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-3 text-sm text-amber-700">没有可用的目标媒体库。</p>
          )}
          {preview && (
            <div className="space-y-2 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-3 text-sm text-emerald-800">
              <p className="font-bold">校验通过</p>
              <div><span className="font-semibold">目标路径：</span><span className="break-all">{preview.result.target_openlist_path}</span></div>
              <div><span className="font-semibold">将移动：</span>{preview.result.media_count} 个媒体项</div>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
          <button type="button" onClick={onClose} className="btn-outline px-4">取消</button>
          <button type="button" onClick={() => void validate()} disabled={!canValidate} className="btn-outline px-4">
            {validating ? <LoaderCircle size={15} className="animate-spin" /> : null}
            {validating ? '校验中…' : '校验目标'}
          </button>
          <button type="button" onClick={() => void apply()} disabled={!preview || applying} className="btn-primary px-5">
            {applying ? <LoaderCircle size={16} className="animate-spin" /> : <ArrowRightLeft size={16} />}
            {applying ? '移动中…' : '确认移动'}
          </button>
        </div>
      </div>
    </div>
  )
}

function apiErrorMessage(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
