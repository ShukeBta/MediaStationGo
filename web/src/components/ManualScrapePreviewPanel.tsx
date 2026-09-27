import { AlertTriangle, Check, CheckCircle2, LoaderCircle, X } from 'lucide-react'
import { useState } from 'react'

import type { ManualScrapeCandidate, ScrapePreviewRow, TMDbScrapeSummary } from '../api/library'

type EpisodeMapping = NonNullable<ManualScrapeCandidate['episode_mappings']>[string]

interface EpisodeMappingDraft {
  season: string
  episode: string
  end: string
  part: string
}

interface ManualScrapePreviewPanelProps {
  title: string
  rows: ScrapePreviewRow[]
  tmdb?: TMDbScrapeSummary
  applying: boolean
  onValidate: (episodeMappings: Record<string, EpisodeMapping>) => void
  onConfirm: () => void
  onCancel: () => void
}

export function ManualScrapePreviewPanel({
  title,
  rows,
  tmdb,
  applying,
  onValidate,
  onConfirm,
  onCancel,
}: ManualScrapePreviewPanelProps) {
  const [drafts, setDrafts] = useState<Record<string, EpisodeMappingDraft>>(() => createMappingDrafts(rows))
  const unresolvedRows = rows.filter((row) => !row.valid)
  const invalidRows = unresolvedRows.filter((row) => mappingDraftError(drafts[row.media_id]))
  const validCount = rows.length - unresolvedRows.length
  const canConfirm = rows.length > 0 && invalidRows.length === 0 && !applying
  const mappedEpisodes = mappedEpisodeCounts(rows, drafts)

  const updateDraft = (mediaID: string, field: keyof EpisodeMappingDraft, value: string) => {
    setDrafts((current) => ({
      ...current,
      [mediaID]: { ...current[mediaID], [field]: value },
    }))
  }

  return (
    <section className="mb-5 overflow-hidden rounded-2xl border border-sand-200 bg-white shadow-sm">
      <div className="flex flex-col gap-3 border-b border-sand-200 bg-sand-50/70 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            {unresolvedRows.length > 0 ? (
              <AlertTriangle className="h-5 w-5 shrink-0 text-amber-500" />
            ) : (
              <CheckCircle2 className="h-5 w-5 shrink-0 text-emerald-500" />
            )}
            <h3 className="truncate font-display text-base font-bold text-ink-600">应用前确认 · {title}</h3>
          </div>
          <p className="mt-1 pl-7 text-xs text-sand-500">
            共 {rows.length} 个媒体，{validCount} 个已识别
            {unresolvedRows.length > 0 ? `，${unresolvedRows.length} 个需要补充季集信息` : '，可以写入'}
          </p>
        </div>
        <button type="button" onClick={onCancel} className="btn-ghost h-9 shrink-0 px-3 text-xs">
          <X size={14} />
          取消
        </button>
      </div>

      {tmdb && (
        <div className="border-b border-sand-200 bg-brand-50/35 px-4 py-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-xs font-bold uppercase tracking-wide text-brand-700">TMDB #{tmdb.tmdb_id} 编目</p>
              <p className="mt-1 text-sm font-semibold text-ink-600">{tmdb.title}</p>
            </div>
            <p className="text-xs font-semibold text-sand-600">
              {tmdb.season_count} 季 · 共 {tmdb.episode_count} 集
            </p>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
            {tmdb.seasons.map((season) => (
              <div key={season.season_num} className="rounded-xl border border-brand-100 bg-white px-3 py-2.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-bold text-ink-600">{season.season_num === 0 ? '特别篇' : `第 ${season.season_num} 季`}</span>
                  <span className="font-mono text-xs font-bold text-brand-700">{season.episode_count} 集</span>
                </div>
                <p className="mt-1 text-[11px] text-sand-500">
                  本次映射 {mappedEpisodes[season.season_num]?.size || 0} 集
                  {season.air_date ? ` · ${season.air_date.slice(0, 4)}` : ''}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="max-h-80 divide-y divide-sand-100 overflow-y-auto">
        {rows.map((row) => {
          const filename = mediaFilename(row.path)
          const draft = drafts[row.media_id]
          const draftError = row.valid ? '' : mappingDraftError(draft)
          return (
            <div key={row.media_id} className={`px-4 py-3 ${row.valid ? 'bg-white' : 'bg-amber-50/45'}`}>
              <div className="flex min-w-0 items-start gap-3">
                <div className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full ${row.valid ? 'bg-emerald-50 text-emerald-600' : 'bg-amber-100 text-amber-700'}`}>
                  {row.valid ? <Check size={14} /> : <AlertTriangle size={14} />}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
                    <p className="truncate text-sm font-semibold text-ink-600" title={filename}>{filename}</p>
                    {row.valid ? (
                      <span className="w-fit shrink-0 rounded-full bg-emerald-50 px-2.5 py-1 font-mono text-xs font-bold text-emerald-700">
                        {episodeLabel(row)}
                      </span>
                    ) : (
                      <span className="w-fit shrink-0 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-bold text-amber-800">需指定</span>
                    )}
                  </div>

                  {!row.valid && (
                    <>
                      <p className="mt-1 text-xs leading-relaxed text-amber-800">{row.error || '无法从路径确认季集号'}</p>
                      <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <EpisodeNumberField label="季" value={draft?.season || ''} min={0} onChange={(value) => updateDraft(row.media_id, 'season', value)} />
                        <EpisodeNumberField label="起始集" value={draft?.episode || ''} min={1} onChange={(value) => updateDraft(row.media_id, 'episode', value)} />
                        <EpisodeNumberField label="结束集（可选）" value={draft?.end || ''} min={1} onChange={(value) => updateDraft(row.media_id, 'end', value)} />
                        <EpisodeNumberField label="分段（可选）" value={draft?.part || ''} min={1} max={99} onChange={(value) => updateDraft(row.media_id, 'part', value)} />
                      </div>
                      {draftError && <p className="mt-2 text-xs font-semibold text-rose-600">{draftError}</p>}
                    </>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>

      <div className="flex flex-col-reverse gap-2 border-t border-sand-200 bg-white px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs text-sand-500">
          {unresolvedRows.length > 0 ? '补齐后先重新校验；校验通过才会开放写入。' : '所有条目均已通过服务端季集校验。'}
        </p>
        <button
          type="button"
          className="btn-primary h-10 shrink-0 justify-center px-5 text-sm"
          disabled={!canConfirm}
          onClick={() => unresolvedRows.length > 0 ? onValidate(buildEpisodeMappings(rows, drafts)) : onConfirm()}
        >
          {applying ? <LoaderCircle size={15} className="animate-spin" /> : <Check size={15} />}
          {unresolvedRows.length > 0 ? `重新校验 ${rows.length} 个媒体` : `确认写入 ${rows.length} 个媒体`}
        </button>
      </div>
    </section>
  )
}

function EpisodeNumberField({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string
  value: string
  min: number
  max?: number
  onChange: (value: string) => void
}) {
  return (
    <label className="grid gap-1 text-[11px] font-bold text-sand-500">
      {label}
      <input
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        step="1"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-9 w-full rounded-lg border border-sand-200 bg-white px-2.5 text-sm font-semibold text-ink-600 outline-none transition focus:border-brand-300 focus:ring-2 focus:ring-brand-100"
      />
    </label>
  )
}

function createMappingDrafts(rows: ScrapePreviewRow[]): Record<string, EpisodeMappingDraft> {
  return Object.fromEntries(rows.map((row) => [row.media_id, {
    season: row.valid || row.episode_num > 0 ? String(row.season_num) : '',
    episode: row.episode_num > 0 ? String(row.episode_num) : '',
    end: row.episode_end_num > 0 ? String(row.episode_end_num) : '',
    part: row.episode_part_num > 0 ? String(row.episode_part_num) : '',
  }]))
}

function mappingDraftError(draft?: EpisodeMappingDraft): string {
  if (!draft?.season.trim() || !draft.episode.trim()) return '请填写季号和起始集号'
  const season = Number(draft.season)
  const episode = Number(draft.episode)
  const end = draft.end.trim() ? Number(draft.end) : 0
  const part = draft.part.trim() ? Number(draft.part) : 0
  if (!Number.isInteger(season) || season < 0 || !Number.isInteger(episode) || episode < 1) return '季号需大于等于 0，集号需大于 0'
  if (!Number.isInteger(end) || (draft.end.trim() && (end < episode || end - episode > 30))) return '结束集号不能小于起始集号，且最多覆盖 31 集'
  if (!Number.isInteger(part) || (draft.part.trim() && (part < 1 || part > 99))) return '分段序号需在 1 至 99 之间'
  return ''
}

function buildEpisodeMappings(rows: ScrapePreviewRow[], drafts: Record<string, EpisodeMappingDraft>): Record<string, EpisodeMapping> {
  return Object.fromEntries(rows.map((row) => {
    const draft = drafts[row.media_id]
    const mapping: EpisodeMapping = {
      season_num: Number(draft.season),
      episode_num: Number(draft.episode),
    }
    if (draft.end.trim()) mapping.episode_end_num = Number(draft.end)
    if (draft.part.trim()) mapping.episode_part_num = Number(draft.part)
    return [row.media_id, mapping]
  }))
}

function episodeLabel(row: ScrapePreviewRow): string {
  const base = `S${padEpisodeNumber(row.season_num)}E${padEpisodeNumber(row.episode_num)}`
  const range = row.episode_end_num > row.episode_num ? `–E${padEpisodeNumber(row.episode_end_num)}` : ''
  const part = row.episode_part_num > 0 ? ` · 第 ${row.episode_part_num} 段` : ''
  return base + range + part
}

function padEpisodeNumber(value: number): string {
  return String(value).padStart(2, '0')
}

function mediaFilename(path: string): string {
  return path.replace(/\\/g, '/').split('/').pop() || path
}

function mappedEpisodeCounts(rows: ScrapePreviewRow[], drafts: Record<string, EpisodeMappingDraft>): Record<number, Set<number>> {
  const result: Record<number, Set<number>> = {}
  for (const row of rows) {
    const draft = drafts[row.media_id]
    if (mappingDraftError(draft)) continue
    const season = Number(draft.season)
    const start = Number(draft.episode)
    const end = draft.end.trim() ? Number(draft.end) : start
    const episodes = result[season] || new Set<number>()
    for (let episode = start; episode <= end; episode++) episodes.add(episode)
    result[season] = episodes
  }
  return result
}
