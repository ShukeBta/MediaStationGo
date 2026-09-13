import { useEffect, useState } from 'react'
import { FolderInput, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { toolsAPI, type OrganizeOverrides } from '../api/tools'
import type { Media } from '../types'

interface OrganizeMediaDialogProps {
  open: boolean
  media: Media | null
  onClose: () => void
  onOrganized?: () => void | Promise<void>
}

// OrganizeMediaDialog 对单个媒体执行「手动整理入库」：把该媒体按命名规范转移到
// 目标媒体目录，并可手动覆盖类型/二级分类/目标路径/转移方式。后端 handler 为
// POST /admin/media/:id/organize（见 organizeMediaHandler）。云盘媒体不支持本地整理。
export function OrganizeMediaDialog({ open, media, onClose, onOrganized }: OrganizeMediaDialogProps) {
  const [form, setForm] = useState({
    media_type: '',
    media_category: '',
    dest_path: '',
    transfer_mode: 'hardlink',
    scan_after: true,
    scrape_after: true,
    dry_run: false,
  })
  const [busy, setBusy] = useState(false)
  const [preview, setPreview] = useState('')

  useEffect(() => {
    if (!open) return
    setPreview('')
    setForm((prev) => ({ ...prev, media_type: '', media_category: '', dest_path: '', dry_run: false }))
  }, [open, media])

  if (!open || !media) return null

  const isCloud = (media.path || '').toLowerCase().startsWith('cloud://')

  const set = (key: keyof typeof form, value: string | boolean) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const run = async (dryRun: boolean) => {
    if (busy) return
    setBusy(true)
    setPreview('')
    try {
      const opts: OrganizeOverrides = {
        transfer_mode: form.transfer_mode,
        scan_after: form.scan_after,
        scrape_after: form.scrape_after,
        dry_run: dryRun,
      }
      if (form.media_type.trim()) opts.media_type = form.media_type.trim()
      if (form.media_category.trim()) opts.media_category = form.media_category.trim()
      if (form.dest_path.trim()) opts.dest_path = form.dest_path.trim()
      const r = await toolsAPI.organizeMedia(media.id, opts)
      if (dryRun) {
        setPreview(r.path || '（未返回目标路径）')
        toast.success('已生成整理预览')
      } else {
        toast.success('整理完成')
        await onOrganized?.()
        onClose()
      }
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '整理失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-8 backdrop-blur-sm">
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="font-display text-xl font-bold text-gray-900">整理入库</h2>
            <p className="mt-1 text-xs text-gray-500">
              把「{media.title}」按命名规范转移到媒体目录。留空则自动识别类型与二级分类。
            </p>
          </div>
          <button onClick={onClose} className="btn-ghost h-9 w-9 p-0" aria-label="关闭">
            <X size={16} />
          </button>
        </div>

        <div className="grid flex-1 gap-4 overflow-y-auto p-5 md:grid-cols-2">
          {isCloud && (
            <div className="md:col-span-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-semibold text-amber-700">
              该媒体位于云盘库，本地整理无法直接移动云盘文件。请在外部存储中启用云盘转移，或对本地媒体使用本功能。
            </div>
          )}
          <label>
            <span className="mb-1 block text-xs font-bold text-gray-500">类型（留空自动识别）</span>
            <select
              value={form.media_type}
              onChange={(e) => set('media_type', e.target.value)}
              className="h-11 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
            >
              <option value="">自动识别</option>
              <option value="movie">电影</option>
              <option value="tv">电视剧</option>
              <option value="anime">动漫</option>
              <option value="variety">综艺</option>
              <option value="adult">成人</option>
            </select>
          </label>
          <Field
            label="二级分类（留空自动识别）"
            value={form.media_category}
            onChange={(v) => set('media_category', v)}
            placeholder="如 华语电影 / 国产剧 / 日番"
          />
          <Field
            label="目标媒体目录（留空用默认）"
            value={form.dest_path}
            onChange={(v) => set('dest_path', v)}
            placeholder="如 /media"
          />
          <label>
            <span className="mb-1 block text-xs font-bold text-gray-500">转移方式</span>
            <select
              value={form.transfer_mode}
              onChange={(e) => set('transfer_mode', e.target.value)}
              className="h-11 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
            >
              <option value="hardlink">硬链接</option>
              <option value="symlink">软链接</option>
              <option value="strm">本地 STRM（跨盘保留源文件）</option>
              <option value="copy">复制</option>
              <option value="move">移动</option>
            </select>
            {form.transfer_mode === 'strm' && (
              <span className="mt-1 block text-xs text-gray-500">生成指向源文件的 STRM，保留媒体和做种；播放时源盘路径须可访问。</span>
            )}
          </label>
          <label className="flex h-11 items-center gap-2 rounded-xl border border-gray-200 px-3 text-sm font-semibold text-gray-700">
            <input
              type="checkbox"
              checked={form.scrape_after}
              onChange={(e) => set('scrape_after', e.target.checked)}
              className="h-4 w-4 rounded border-gray-300 text-brand-600"
            />
            整理后刮削
          </label>
          <label className="flex h-11 items-center gap-2 rounded-xl border border-gray-200 px-3 text-sm font-semibold text-gray-700">
            <input
              type="checkbox"
              checked={form.scan_after}
              onChange={(e) => set('scan_after', e.target.checked)}
              className="h-4 w-4 rounded border-gray-300 text-brand-600"
            />
            整理后扫描入库
          </label>

          {preview && (
            <div className="md:col-span-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-700">
              <span className="font-bold text-gray-500">预览目标路径：</span>
              <span className="break-all">{preview}</span>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
          <button onClick={onClose} className="btn-outline px-4">取消</button>
          <button onClick={() => run(true)} disabled={busy || isCloud} className="btn-outline px-4">
            预览路径
          </button>
          <button onClick={() => run(false)} disabled={busy || isCloud} className="btn-primary px-5">
            <FolderInput size={16} />
            {busy ? '整理中…' : '整理入库'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
}) {
  return (
    <label>
      <span className="mb-1 block text-xs font-bold text-gray-500">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-11 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
      />
    </label>
  )
}
