import { FormEvent, useEffect, useState } from 'react'
import { Image, LoaderCircle, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { mediaAPI } from '../api/library'
import type { Media } from '../types'

type GeneratedArtworkDialogProps = {
  open: boolean
  media: Media
  onClose: () => void
  onGenerated: (media: Media) => void | Promise<void>
}

export function GeneratedArtworkDialog({ open, media, onClose, onGenerated }: GeneratedArtworkDialogProps) {
  const [timestamp, setTimestamp] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setTimestamp(formatTimestamp(media.generated_artwork_seek_sec || defaultPreviewSeek(media.duration_sec)))
    setError('')
  }, [media.duration_sec, media.generated_artwork_seek_sec, open])

  if (!open) return null

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const seconds = parseTimestamp(timestamp)
    if (seconds == null || seconds < 1) {
      setError('请输入有效时间点，最早为 00:00:01')
      return
    }
    if (media.duration_sec <= 0) {
      setError('当前作品缺少时长，请先探测媒体轨')
      return
    }
    if (seconds >= media.duration_sec - 1) {
      setError(`时间点必须早于 ${formatTimestamp(media.duration_sec - 1)}`)
      return
    }
    setBusy(true)
    setError('')
    try {
      const next = await mediaAPI.generateArtworkAt(media.id, seconds)
      await onGenerated(next)
      toast.success(`已使用 ${formatTimestamp(seconds)} 生成预览图`)
      onClose()
    } catch (requestError: unknown) {
      const message = (requestError as { response?: { data?: { error?: string } } })?.response?.data?.error
      setError(message || '生成预览图失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
      <form onSubmit={submit} className="w-full max-w-md overflow-hidden rounded-lg border border-gray-200 bg-white shadow-2xl">
        <header className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
          <div className="flex min-w-0 items-center gap-2">
            <Image size={18} className="shrink-0 text-brand-500" />
            <div className="min-w-0">
              <h2 className="truncate text-base font-semibold text-ink-600">按时间生成预览图</h2>
              <p className="truncate text-xs text-ink-50">{media.title}</p>
            </div>
          </div>
          <button type="button" className="icon-button" onClick={onClose} disabled={busy} title="关闭">
            <X size={16} />
          </button>
        </header>

        <div className="space-y-3 p-4">
          <label className="block text-sm font-medium text-ink-600">
            截取时间点
            <input
              autoFocus
              className="input-base mt-1 text-base"
              value={timestamp}
              onChange={(event) => setTimestamp(event.target.value)}
              placeholder="00:05:30.500"
              inputMode="decimal"
            />
          </label>
          <div className="flex items-center justify-between text-xs text-ink-50">
            <span>格式 HH:MM:SS，可精确到毫秒</span>
            <span className="tabular-nums">总时长 {formatTimestamp(media.duration_sec)}</span>
          </div>
          {error && <p className="rounded-lg border border-red-200 bg-red-50 p-2 text-sm text-red-600">{error}</p>}
        </div>

        <footer className="flex justify-end gap-2 border-t border-gray-200 px-4 py-3">
          <button type="button" className="btn-outline px-4" onClick={onClose} disabled={busy}>取消</button>
          <button type="submit" className="btn-primary px-4" disabled={busy}>
            {busy ? <LoaderCircle size={15} className="animate-spin" /> : <Image size={15} />}
            {busy ? '生成中…' : '生成预览图'}
          </button>
        </footer>
      </form>
    </div>
  )
}

function parseTimestamp(value: string): number | null {
  const normalized = value.trim()
  if (!normalized) return null
  if (!normalized.includes(':')) {
    const seconds = Number(normalized)
    return Number.isFinite(seconds) ? seconds : null
  }
  const parts = normalized.split(':')
  if (parts.length < 2 || parts.length > 3 || parts.some((part) => part.trim() === '')) return null
  const values = parts.map(Number)
  if (values.some((part) => !Number.isFinite(part) || part < 0)) return null
  const seconds = values[values.length - 1] ?? 0
  const minutes = values[values.length - 2] ?? 0
  const hours = values.length === 3 ? values[0] : 0
  if (seconds >= 60 || (values.length === 3 && minutes >= 60)) return null
  return hours * 3600 + minutes * 60 + seconds
}

function formatTimestamp(value: number): string {
  const safe = Math.max(0, Number.isFinite(value) ? value : 0)
  const hours = Math.floor(safe / 3600)
  const minutes = Math.floor((safe % 3600) / 60)
  const seconds = safe % 60
  const secondsText = seconds % 1 === 0
    ? String(Math.floor(seconds)).padStart(2, '0')
    : seconds.toFixed(3).padStart(6, '0').replace(/0+$/, '').replace(/\.$/, '')
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${secondsText}`
}

function defaultPreviewSeek(durationSec: number): number {
  if (durationSec <= 0) return 10
  let seek = durationSec * 0.1
  if (durationSec >= 120 && seek < 60) seek = 60
  if (seek > 300) seek = 300
  if (seek >= durationSec - 5) seek = durationSec / 2
  return Math.max(1, seek)
}
