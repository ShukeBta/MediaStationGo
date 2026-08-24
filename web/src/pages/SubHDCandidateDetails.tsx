import { CalendarDays, CircleCheck, Download, ExternalLink, ThumbsUp, User } from 'lucide-react'

import type { SubtitleSearchCandidate } from '../api/subtitles'

export function SubHDCandidateDetails({ candidate, applied = false }: { candidate: SubtitleSearchCandidate; applied?: boolean }) {
  const tags = [candidate.source_type, ...candidate.language_tags, ...candidate.formats, candidate.subtitle_group].filter(Boolean)
  return (
    <div className="mt-3 border-t border-sand-100 pt-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        {candidate.url ? (
          <a href={candidate.url} target="_blank" rel="noreferrer" className="break-words text-sm font-medium text-ink-600 hover:text-[#9b6a2f] hover:underline">
            {candidate.title || candidate.filename} <ExternalLink size={12} className="inline" />
          </a>
        ) : <p className="break-words text-sm font-medium text-ink-600">{candidate.title || candidate.filename}</p>}
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          {applied && <span className="inline-flex items-center gap-1 rounded bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700"><CircleCheck size={13} />当前已应用</span>}
          {candidate.can_preview === false && <span className="rounded bg-amber-50 px-2 py-1 text-xs text-amber-700" title={candidate.preview_unavailable_reason}>图形字幕，无文本预览</span>}
        </div>
      </div>
      {tags.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {tags.map((tag, index) => <span key={`${tag}-${index}`} className="rounded bg-sand-100 px-2 py-1 text-xs text-sand-600">{tag}</span>)}
        </div>
      )}
      <div className="mt-3 grid gap-2 text-xs text-sand-600 sm:grid-cols-2 lg:grid-cols-4">
        <span className="flex items-center gap-1.5"><Download size={13} /> 下载 {candidate.download_count.toLocaleString()}</span>
        <span className="flex items-center gap-1.5"><ThumbsUp size={13} /> 点赞 {candidate.like_count.toLocaleString()}</span>
        <span className="flex items-center gap-1.5"><User size={13} /> {candidate.uploader || '上传人未标注'}</span>
        <span className="flex items-center gap-1.5"><CalendarDays size={13} /> {candidate.uploaded_date || candidate.uploaded_at || '日期未标注'}</span>
      </div>
    </div>
  )
}
