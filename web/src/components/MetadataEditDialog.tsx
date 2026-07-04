import { useEffect, useState } from 'react'
import { Save, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { mediaAPI, type MediaMetadataUpdate } from '../api/library'
import type { Media } from '../types'

interface MetadataEditDialogProps {
  open: boolean
  media: Media | null
  mediaIds?: string[]
  mode?: 'media' | 'series'
  scopeLabel?: string
  onClose: () => void
  onSaved: (media: Media) => void | Promise<void>
}

export function MetadataEditDialog({
  open,
  media,
  mediaIds,
  mode = 'media',
  scopeLabel,
  onClose,
  onSaved,
}: MetadataEditDialogProps) {
  const [form, setForm] = useState({
    title: '',
    original_name: '',
    overview: '',
    poster_url: '',
    backdrop_url: '',
    year: '',
    release_date: '',
    rating: '',
    season_num: '',
    episode_num: '',
    tmdb_id: '',
    bangumi_id: '',
    douban_id: '',
    thetvdb_id: '',
    languages: '',
    countries: '',
    genres: '',
    nsfw: false,
  })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !media) return
    setForm({
      title: media.title || '',
      original_name: media.original_name || '',
      overview: media.overview || '',
      poster_url: media.poster_url || '',
      backdrop_url: media.backdrop_url || '',
      year: media.year > 0 ? String(media.year) : '',
      release_date: media.release_date || '',
      rating: media.rating > 0 ? String(media.rating) : '',
      season_num: media.season_num > 0 || media.episode_num > 0 ? String(media.season_num || 0) : '',
      episode_num: media.episode_num > 0 ? String(media.episode_num) : '',
      tmdb_id: media.tmdb_id > 0 ? String(media.tmdb_id) : '',
      bangumi_id: media.bangumi_id > 0 ? String(media.bangumi_id) : '',
      douban_id: media.douban_id || '',
      thetvdb_id: media.thetvdb_id || '',
      languages: media.languages || '',
      countries: media.countries || '',
      genres: media.genres || '',
      nsfw: !!media.nsfw,
    })
  }, [open, media])

  if (!open || !media) return null

  const targetIds = Array.from(new Set((mediaIds && mediaIds.length > 0 ? mediaIds : [media.id]).filter(Boolean)))
  const isSeries = mode === 'series' && targetIds.length > 1

  const set = (key: keyof typeof form, value: string | boolean) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }
  const toNumber = (value: string) => {
    const trimmed = value.trim()
    if (!trimmed) return 0
    const parsed = Number(trimmed)
    return Number.isFinite(parsed) ? parsed : 0
  }
  const buildPayload = (): MediaMetadataUpdate => {
    const payload: MediaMetadataUpdate = {
      title: form.title,
      overview: form.overview,
      poster_url: form.poster_url,
      backdrop_url: form.backdrop_url,
      year: Math.trunc(toNumber(form.year)),
      release_date: form.release_date,
      rating: toNumber(form.rating),
      tmdb_id: Math.trunc(toNumber(form.tmdb_id)),
      bangumi_id: Math.trunc(toNumber(form.bangumi_id)),
      douban_id: form.douban_id,
      thetvdb_id: form.thetvdb_id,
      languages: form.languages,
      countries: form.countries,
      genres: form.genres,
      nsfw: form.nsfw,
    }
    if (!isSeries) {
      payload.original_name = form.original_name
      payload.season_num = Math.trunc(toNumber(form.season_num))
      payload.episode_num = Math.trunc(toNumber(form.episode_num))
    }
    return payload
  }
  const save = async () => {
    if (!form.title.trim()) {
      toast.error('标题不能为空')
      return
    }
    setSaving(true)
    try {
      const payload = buildPayload()
      let next: Media | null = null
      for (const id of targetIds) {
        const updated = await mediaAPI.updateMetadata(id, payload)
        if (!next || id === media.id) next = updated
      }
      if (!next) next = await mediaAPI.updateMetadata(media.id, payload)
      toast.success(isSeries ? `整剧元数据已保存：${targetIds.length} 集` : '元数据已保存')
      await onSaved(next)
      onClose()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-8 backdrop-blur-sm">
      <div className="flex max-h-[88vh] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="font-display text-xl font-bold text-gray-900">
              {isSeries ? '编辑整剧元数据' : '编辑元数据'}
            </h2>
            <p className="mt-1 text-xs text-gray-500">{scopeLabel || '用于手动修正自采集或无法自动匹配的媒体。'}</p>
          </div>
          <button onClick={onClose} className="btn-ghost h-9 w-9 p-0" aria-label="关闭">
            <X size={16} />
          </button>
        </div>
        <div className="grid flex-1 gap-4 overflow-y-auto p-5 md:grid-cols-2">
          <Field label="标题" value={form.title} onChange={(value) => set('title', value)} />
          {!isSeries && <Field label="原名 / 单集名" value={form.original_name} onChange={(value) => set('original_name', value)} />}
          <Field label="海报 URL" value={form.poster_url} onChange={(value) => set('poster_url', value)} />
          <Field label="背景 / 单集剧照 URL" value={form.backdrop_url} onChange={(value) => set('backdrop_url', value)} />
          <Field label="年份" value={form.year} onChange={(value) => set('year', value)} inputMode="numeric" />
          <Field label="上映日期" value={form.release_date} onChange={(value) => set('release_date', value)} type="date" />
          <Field label="评分" value={form.rating} onChange={(value) => set('rating', value)} inputMode="decimal" />
          {!isSeries && <Field label="季" value={form.season_num} onChange={(value) => set('season_num', value)} inputMode="numeric" />}
          {!isSeries && <Field label="集" value={form.episode_num} onChange={(value) => set('episode_num', value)} inputMode="numeric" />}
          <Field label="TMDb ID" value={form.tmdb_id} onChange={(value) => set('tmdb_id', value)} inputMode="numeric" />
          <Field label="Bangumi ID" value={form.bangumi_id} onChange={(value) => set('bangumi_id', value)} inputMode="numeric" />
          <Field label="豆瓣 ID" value={form.douban_id} onChange={(value) => set('douban_id', value)} />
          <Field label="TheTVDB ID" value={form.thetvdb_id} onChange={(value) => set('thetvdb_id', value)} />
          <Field label="语言" value={form.languages} onChange={(value) => set('languages', value)} placeholder="zh,en" />
          <Field label="国家/地区" value={form.countries} onChange={(value) => set('countries', value)} placeholder="CN,JP,US" />
          <Field label="类型" value={form.genres} onChange={(value) => set('genres', value)} placeholder="剧情,动画" />
          <label className="flex h-11 items-center gap-2 rounded-xl border border-gray-200 px-3 text-sm font-semibold text-gray-700">
            <input
              type="checkbox"
              checked={form.nsfw}
              onChange={(event) => set('nsfw', event.target.checked)}
              className="h-4 w-4 rounded border-gray-300 text-brand-600"
            />
            成人内容
          </label>
          <label className="md:col-span-2">
            <span className="mb-1 block text-xs font-bold text-gray-500">简介</span>
            <textarea
              value={form.overview}
              onChange={(event) => set('overview', event.target.value)}
              rows={5}
              className="w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
            />
          </label>
        </div>
        <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
          <button onClick={onClose} className="btn-outline px-4">取消</button>
          <button onClick={save} disabled={saving} className="btn-primary px-5">
            <Save size={16} />
            保存
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
  inputMode,
  type = 'text',
}: {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  inputMode?: 'numeric' | 'decimal'
  type?: 'text' | 'date'
}) {
  return (
    <label>
      <span className="mb-1 block text-xs font-bold text-gray-500">{label}</span>
      <input
        value={value}
        type={type}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        inputMode={inputMode}
        className="h-11 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-semibold text-gray-700 outline-none focus:border-brand-300"
      />
    </label>
  )
}
