import { Layers, LoaderCircle, Play } from 'lucide-react'
import { Link } from 'react-router-dom'

import type { MediaPart } from '../types'
import { formatSize } from './libraryPageModel'

type MediaDetailPartsProps = {
  parts: MediaPart[]
  loading: boolean
}

export function MediaDetailParts({ parts, loading }: MediaDetailPartsProps) {
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-sand-500">
        <LoaderCircle size={15} className="animate-spin" />
        正在加载作品片段
      </div>
    )
  }
  if (parts.length <= 1) return null

  return (
    <section className="space-y-3" aria-label="作品片段">
      <div className="flex items-center gap-2">
        <Layers size={15} className="text-[#c9954a]" />
        <h2 className="text-sm font-semibold text-ink-600">作品片段</h2>
        <span className="text-xs text-sand-500">{parts.length} 个</span>
      </div>
      <div className="divide-y divide-gray-200 border-y border-gray-200">
        {parts.map((part) => (
          <div key={part.id} className="grid gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium text-ink-600">
                <Link to={`/media/${part.id}`} className="hover:text-brand-500">
                  {partLabel(part)}
                </Link>
                {part.is_current && <span className="text-xs font-normal text-[#c9954a]">当前片段</span>}
              </div>
              {part.path && <p className="truncate text-xs text-sand-500" title={part.path}>{part.path}</p>}
              <p className="text-xs text-ink-50">{partTechnicalText(part)}</p>
            </div>
            <Link
              to={`/play/${part.id}`}
              title={`播放${partLabel(part)}`}
              aria-label={`播放${partLabel(part)}`}
              className="btn-outline h-9 w-9 justify-center p-0"
            >
              <Play size={14} fill="currentColor" />
            </Link>
          </div>
        ))}
      </div>
    </section>
  )
}

function partLabel(part: MediaPart): string {
  const prefix = part.part_index && part.part_index > 0 ? `第 ${part.part_index} 段` : '片段'
  return `${prefix} · ${part.title}`
}

function partTechnicalText(part: MediaPart): string {
  return [
    part.size_bytes > 0 ? formatSize(part.size_bytes) : '',
    part.container?.toUpperCase(),
    part.duration_sec > 0 ? `${Math.round(part.duration_sec / 60)} 分钟` : '',
  ].filter(Boolean).join(' · ')
}
