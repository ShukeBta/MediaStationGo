import { useRef, useState } from 'react'
import toast from 'react-hot-toast'
import type { Media } from '../types'
import { mediaAPI, type ManualScrapeCandidate, type ScrapePreviewRow } from '../api/library'
import {
  ManualScrapeCandidateList,
  ManualScrapeDialogHeader,
  ManualScrapeSearchControls,
} from './ManualScrapeDialogSections'
import { useManualScrapeDialogState } from './useManualScrapeDialogState'

interface ManualScrapeDialogProps {
  open: boolean
  media: Media | null
  mediaIds?: string[]
  defaultQuery?: string
  mediaType?: string
  scopeLabel?: string
  onClose: () => void
  onApplied?: () => void
}

export function ManualScrapeDialog({
  open,
  media,
  mediaIds,
  defaultQuery,
  mediaType,
  scopeLabel,
  onClose,
  onApplied,
}: ManualScrapeDialogProps) {
  const [episodeOverride, setEpisodeOverride] = useState({ mediaId: '', season: '', episode: '' })
  const [coverage, setCoverage] = useState({ mediaId: '', end: '', part: '' })
  const [preview, setPreview] = useState<{ mediaId: string; item: ManualScrapeCandidate; rows: ScrapePreviewRow[] } | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const previewGeneration = useRef(0)
  const closeDialog = () => {
    previewGeneration.current++
    setPreviewing(false)
    setPreview(null)
    setCoverage({ mediaId: '', end: '', part: '' })
    setEpisodeOverride({ mediaId: '', season: '', episode: '' })
    onClose()
  }
  const dialog = useManualScrapeDialogState({
    open,
    media,
    mediaIds,
    defaultQuery,
    mediaType,
    onClose: closeDialog,
    onApplied,
  })

  if (!open || !media) return null

  const previewMatch = async (item: ManualScrapeCandidate) => {
    const generation = ++previewGeneration.current
    setPreview(null)
    setPreviewing(true)
    try {
      const result = await mediaAPI.previewManualScrape(dialog.targetIds, item)
      if (generation !== previewGeneration.current) return
      setPreview({ mediaId: media.id, item, rows: result.items })
    } catch (error) {
      toast.error((error as { response?: { data?: { error?: string } } }).response?.data?.error || '预览失败，未写入数据')
    } finally { if (generation === previewGeneration.current) setPreviewing(false) }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-8 backdrop-blur-sm">
      <div className="flex max-h-[86vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-sand-200 bg-white shadow-2xl">
        <ManualScrapeDialogHeader title={scopeLabel || media.title} targetCount={dialog.targetIds.length} onClose={closeDialog} />

        <ManualScrapeSearchControls
          query={dialog.query}
          selectedProviders={dialog.selectedProviders}
          searching={dialog.searching}
          onQueryChange={dialog.setQuery}
          onProviderChange={dialog.setSelectedProviders}
          onSearch={dialog.runSearch}
        />

        <div className="flex-1 overflow-y-auto p-5">
          <p className="mb-4 text-sm text-ink-600">
            手动选择以所选条目为准，即使剧名与路径不同也可应用。剧集季集号仍会结合路径与 TMDB 校验；冲突时请核对并修正季集信息后重试。
          </p>
          {dialog.targetIds.length === 1 && (
            <div className="mb-4 flex flex-wrap items-center gap-3 text-sm" onChange={() => { previewGeneration.current++; setPreviewing(false); setPreview(null) }}>
              <span>季集冲突时可手动指定（留空按路径校验）：</span>
              <input aria-label="指定季号" type="number" min="0" step="1" placeholder="季号"
                className="w-24 rounded border border-sand-200 px-2 py-1"
                value={episodeOverride.mediaId === media.id ? episodeOverride.season : ''}
                onChange={(e) => setEpisodeOverride({ mediaId: media.id, season: e.target.value, episode: episodeOverride.mediaId === media.id ? episodeOverride.episode : '' })} />
              <input aria-label="指定集号" type="number" min="1" step="1" placeholder="集号"
                className="w-24 rounded border border-sand-200 px-2 py-1"
                value={episodeOverride.mediaId === media.id ? episodeOverride.episode : ''}
                onChange={(e) => setEpisodeOverride({ mediaId: media.id, episode: e.target.value, season: episodeOverride.mediaId === media.id ? episodeOverride.season : '' })} />
              <input aria-label="合并集结束集号" type="number" min="1" step="1" placeholder="结束集号（可选）" className="w-40 rounded border border-sand-200 px-2 py-1"
                value={coverage.mediaId === media.id ? coverage.end : ''} onChange={e => { setPreview(null); setCoverage({ mediaId: media.id, end: e.target.value, part: coverage.mediaId === media.id ? coverage.part : '' }) }} />
              <input aria-label="分段序号" type="number" min="1" max="99" step="1" placeholder="分段序号（可选）" className="w-40 rounded border border-sand-200 px-2 py-1"
                value={coverage.mediaId === media.id ? coverage.part : ''} onChange={e => { setPreview(null); setCoverage({ mediaId: media.id, part: e.target.value, end: coverage.mediaId === media.id ? coverage.end : '' }) }} />
            </div>
          )}
          {previewing && <p role="status">正在按剧、季校验，不写入数据…</p>}
          {preview?.mediaId === media.id && <div className="mb-4 rounded border border-sand-200 p-3">
            <p>确认应用《{preview.item.title}》；以下是服务端校验结果：</p>
            <div className="max-h-48 overflow-auto">{preview.rows.map(row => <p key={row.media_id} className="my-2 break-all text-sm">
              {row.path.split('/').pop()}：{row.valid ? `S${row.season_num} E${row.episode_num}${row.episode_end_num ? `–${row.episode_end_num}` : ''}${row.episode_part_num ? ` 第${row.episode_part_num}段` : ''}` : row.error}
            </p>)}</div>
            <button type="button" className="rounded border px-3 py-1 disabled:opacity-50" disabled={!!dialog.applyingKey || !preview.rows.length || preview.rows.some(row => !row.valid)}
              onClick={() => void dialog.apply({ ...preview.item, expected_revisions: Object.fromEntries(preview.rows.map(row => [row.media_id, row.revision])) })}>确认写入</button>
            <button type="button" className="ml-3" onClick={() => setPreview(null)}>取消预览</button>
          </div>}
          <ManualScrapeCandidateList items={dialog.items} applyingKey={dialog.applyingKey} onApply={(item) => {
            if (previewing || dialog.applyingKey) return
            if (coverage.mediaId === media.id && (coverage.end || coverage.part) && !(episodeOverride.mediaId === media.id && episodeOverride.season && episodeOverride.episode)) {
              toast.error('合并集或分段必须同时填写季号与起始集号'); return
            }
            if (dialog.targetIds.length === 1 && episodeOverride.mediaId === media.id && (episodeOverride.season || episodeOverride.episode)) {
              const season = Number(episodeOverride.season)
              const episode = Number(episodeOverride.episode)
              if (!episodeOverride.season || !episodeOverride.episode || !Number.isInteger(season) || !Number.isInteger(episode) || season < 0 || episode < 1) {
                toast.error('请同时填写有效的季号和集号')
                return
              }
              const end = coverage.mediaId === media.id && coverage.end ? Number(coverage.end) : 0
              const part = coverage.mediaId === media.id && coverage.part ? Number(coverage.part) : 0
              if (!Number.isInteger(end) || !Number.isInteger(part) || (end !== 0 && (end < episode || end - episode > 30)) || part < 0 || part > 99) { toast.error('请填写有效的结束集号和分段序号'); return }
              void previewMatch({ ...item, season_num: season, episode_num: episode, episode_end_num: end, episode_part_num: part })
              return
            }
            void previewMatch(item)
          }} />
        </div>
      </div>
    </div>
  )
}
