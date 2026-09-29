import type { ReactNode } from 'react'
import { Clock3, Monitor, Star } from 'lucide-react'
import type { Media } from '../types'

type MediaDetailMetadataProps = { media: Media; children?: ReactNode }

export function MediaDetailMetadata({ media, children }: MediaDetailMetadataProps) {
  const baseTitle = media.display_title?.trim() || media.title
  const heading = media.episode_title?.trim() || baseTitle
  const showTitleContext = Boolean(media.episode_title?.trim() && baseTitle && baseTitle !== heading)
  const videoFormat = [media.video_codec?.toUpperCase(), media.video_profile, media.video_bit_depth ? `${media.video_bit_depth}bit` : ''].filter(Boolean).join(' · ')
  const audioFormat = [media.audio_codec?.toUpperCase(), media.audio_channels ? `${media.audio_channels}ch` : '', media.audio_sample_rate ? `${Math.round(media.audio_sample_rate / 1000)}kHz` : ''].filter(Boolean).join(' · ')

  return (
    <>
      <div className="media-detail-heading">
        <p className="media-eyebrow">{media.episode_num > 0 ? 'EPISODE / 剧集' : 'IN YOUR COLLECTION / 影片详情'}</p>
        <h1>{heading}</h1>
        {showTitleContext && <p className="media-detail-context">{baseTitle}</p>}
        <div className="media-detail-facts">
          {media.rating > 0 && <span className="media-detail-rating"><Star size={13} fill="currentColor" />{media.rating.toFixed(1)}</span>}
          {media.year > 0 && <span>{media.year} 年</span>}
          {media.duration_sec > 0 && <span><Clock3 size={13} />{fmtDuration(media.duration_sec)}</span>}
          {media.video_range && <span className="media-detail-chip">{media.video_range}</span>}
          {media.adult_type && <span className="media-detail-chip">{media.adult_type}</span>}
          {media.douban_id && <a href={`https://movie.douban.com/subject/${encodeURIComponent(media.douban_id)}/`} target="_blank" rel="noreferrer" className="media-detail-douban" title={media.douban_fetched_at ? `更新于 ${new Date(media.douban_fetched_at).toLocaleString()}` : undefined}>
            豆瓣 {(media.douban_rating ?? 0) > 0 ? media.douban_rating!.toFixed(1) : '详情'}{media.douban_degraded ? ' · 基础详情' : ''}
          </a>}
        </div>
      </div>
      {children}
      {media.overview && <section className="media-detail-overview"><h2>关于这部作品</h2><p>{media.overview}</p></section>}
      <div className="media-detail-credits">
        <MetadataTags label="演员" values={parseCSV(media.actors)} primary />
        <MetadataTags label="类型" values={parseCSV(media.genres)} primary />
        <MetadataTags label="语言" values={parseCSV(media.languages)} />
        <MetadataTags label="国家/地区" values={parseCSV(media.countries)} />
      </div>
      <details className="media-technical-details">
        <summary><Monitor size={14} /><span>媒体信息</span><span className="media-technical-summary">{media.container?.toUpperCase()}{media.width > 0 ? ` · ${media.width} × ${media.height}` : ''}</span></summary>
        <div className="media-technical-grid">
          <TechnicalFact label="文件大小" value={fmtSize(media.size_bytes)} />
          <TechnicalFact label="时长" value={fmtDuration(media.duration_sec)} />
          {media.width > 0 && <TechnicalFact label="分辨率" value={`${media.width} × ${media.height}`} />}
          {media.container && <TechnicalFact label="封装" value={media.container.toUpperCase()} />}
          {videoFormat && <TechnicalFact label="视频编码" value={videoFormat} />}
          {media.video_range && <TechnicalFact label="动态范围" value={media.video_range} />}
          {media.frame_rate && media.frame_rate > 0 ? <TechnicalFact label="帧率" value={`${media.frame_rate.toFixed(3).replace(/\.000$/, '')} FPS`} /> : null}
          {media.bit_rate && media.bit_rate > 0 ? <TechnicalFact label="码率" value={fmtBitRate(media.bit_rate)} /> : null}
          {audioFormat && <TechnicalFact label="音频" value={audioFormat} />}
        </div>
      </details>
    </>
  )
}

function TechnicalFact({ label, value }: { label: string; value: string }) {
  return <div><span>{label}</span><strong>{value}</strong></div>
}

function MetadataTags({ label, values, primary = false }: { label: string; values: string[]; primary?: boolean }) {
  if (values.length === 0) return null
  return <div className="media-credit-row"><span>{label}</span><div>{values.map((value) => <span key={value} className={primary ? 'media-credit-primary' : ''}>{value}</span>)}</div></div>
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
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
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
