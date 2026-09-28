import { Captions, Music2, Video } from 'lucide-react'
import type { Media } from '../types'

const trackKinds = [
  { type: 'video', label: '视频', Icon: Video },
  { type: 'audio', label: '音频', Icon: Music2 },
  { type: 'subtitle', label: '字幕', Icon: Captions },
] as const

export function MediaDetailTracks({ media }: { media: Media }) {
  if (!media.tracks?.length) return null
  return (
    <section className="space-y-3" aria-label="媒体轨道">
      <h2 className="text-sm font-bold">媒体轨道</h2>
      <div className="grid gap-3">
        {trackKinds.map(({ type, label, Icon }) => {
          const tracks = media.tracks!.filter((track) => track.type === type)
          if (!tracks.length) return null
          return (
            <div key={type} className="rounded-xl border border-gray-200/60 p-3 dark:border-white/10">
              <h3 className="mb-2 flex items-center gap-2 text-xs font-semibold text-gray-500"><Icon size={15} />{label} · {tracks.length}</h3>
              <ul className="space-y-2 text-sm">
                {tracks.map((track) => (
                  <li key={track.index} className="break-words">
                    <span className="mr-2 text-gray-400">#{track.index}</span>
                    {track.display_title || track.title || track.codec || label}
                    {track.bit_rate ? <span className="ml-2 text-xs text-gray-500">{(track.bit_rate / 1000).toFixed(0)} kbps</span> : null}
                    {track.sample_rate ? <span className="ml-2 text-xs text-gray-500">{track.sample_rate / 1000} kHz</span> : null}
                  </li>
                ))}
              </ul>
            </div>
          )
        })}
      </div>
      <p className="text-xs text-gray-500">音轨与字幕可在播放器中选择。</p>
    </section>
  )
}
