import { Calendar } from 'lucide-react'

import type { Media } from '../types'

type MediaDetailMetadataProps = {
  media: Media
}

export function MediaDetailMetadata({ media }: MediaDetailMetadataProps) {
  const baseTitle = media.display_title?.trim() || media.title
  const heading = media.episode_title?.trim() || baseTitle
  const showTitleContext = Boolean(media.episode_title?.trim() && baseTitle && baseTitle !== heading)

  return (
    <>
      <div className="space-y-3">
        <h1 className="font-display text-3xl sm:text-4xl font-extrabold tracking-tight text-gray-900 leading-tight">
          {heading}
        </h1>
        {showTitleContext && (
          <p className="text-sm font-semibold text-gray-500">
            {baseTitle}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-2.5 text-xs text-gray-500 font-bold tracking-wide uppercase">
          {media.adult_type && (
            <span className="inline-flex items-center rounded-xl border border-brand-200 bg-brand-50 px-2.5 py-1 text-brand-700">
              {media.adult_type}
            </span>
          )}
          {media.year > 0 && (
            <span className="inline-flex items-center gap-1 bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
              <Calendar size={13} className="text-brand-500" />
              <span>{media.year} 年</span>
            </span>
          )}
          {media.width > 0 && (
            <span className="inline-flex items-center gap-1 bg-brand-50 text-brand-700 border border-brand-100/50 px-2.5 py-1 rounded-xl">
              <span>{media.width} × {media.height}</span>
            </span>
          )}
          <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
            {fmtSize(media.size_bytes)}
          </span>
          <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
            {fmtDuration(media.duration_sec)}
          </span>
          {media.container && (
            <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700 font-mono">
              {media.container}
            </span>
          )}
          {media.video_codec && (
            <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
              {[media.video_codec.toUpperCase(), media.video_profile, media.video_bit_depth ? `${media.video_bit_depth}bit` : ''].filter(Boolean).join(' · ')}
            </span>
          )}
          {media.video_range && (
            <span className="bg-gray-900 px-2.5 py-1 rounded-xl text-white">
              {media.video_range}
            </span>
          )}
          {media.frame_rate && media.frame_rate > 0 && (
            <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
              {media.frame_rate.toFixed(3).replace(/\.000$/, '')} FPS
            </span>
          )}
          {media.bit_rate && media.bit_rate > 0 && (
            <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
              {fmtBitRate(media.bit_rate)}
            </span>
          )}
          {media.audio_codec && (
            <span className="bg-gray-100 border border-gray-200/50 px-2.5 py-1 rounded-xl text-gray-700">
              {[media.audio_codec.toUpperCase(), media.audio_channels ? `${media.audio_channels}ch` : '', media.audio_sample_rate ? `${Math.round(media.audio_sample_rate / 1000)}kHz` : ''].filter(Boolean).join(' · ')}
            </span>
          )}
        </div>
      </div>

      {media.overview && (
        <div className="rounded-2xl bg-gray-50/50 border border-gray-100 p-5 space-y-2">
          <h3 className="text-xs font-bold uppercase tracking-widest text-brand-500">剧情简介</h3>
          <p className="text-sm text-gray-600 leading-relaxed font-semibold">
            {media.overview}
          </p>
        </div>
      )}

      <div className="space-y-4">
        <MetadataTags label="演员" values={parseCSV(media.actors)} primary />
        <MetadataTags label="类型流派" values={parseCSV(media.genres)} primary />
        <MetadataTags label="语言" values={parseCSV(media.languages)} />
        <MetadataTags label="国家/地区" values={parseCSV(media.countries)} />
      </div>
    </>
  )
}

function MetadataTags({ label, values, primary = false }: { label: string; values: string[]; primary?: boolean }) {
  if (values.length === 0) return null
  const tagClass = primary
    ? 'rounded-full bg-brand-50 text-brand-700 border border-brand-100/30 px-3 py-1 text-2xs font-bold uppercase tracking-wider'
    : 'rounded-xl bg-gray-100 text-gray-600 border border-gray-200/40 px-2.5 py-1 text-2xs font-semibold'
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span className="text-xs font-bold text-gray-500 w-16 uppercase tracking-wider">{label}</span>
      <div className="flex flex-wrap gap-2">
        {values.map((value) => (
          <span key={value} className={tagClass}>
            {value}
          </span>
        ))}
      </div>
    </div>
  )
}

function fmtDuration(sec: number): string {
  if (!sec || sec <= 0) return '—'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

function fmtSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(2)} ${units[i]}`
}

function fmtBitRate(bits: number): string {
  if (!bits || bits <= 0) return ''
  return bits >= 1_000_000 ? `${(bits / 1_000_000).toFixed(1)} Mbps` : `${Math.round(bits / 1000)} Kbps`
}

function parseCSV(s?: string): string[] {
  if (!s) return []
  return s.split(',').map((x) => x.trim()).filter(Boolean)
}
