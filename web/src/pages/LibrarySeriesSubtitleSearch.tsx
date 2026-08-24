import { useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { Captions, Check, ChevronDown, ExternalLink, Eye, LoaderCircle, RotateCcw, Search, X } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  subtitlesAPI,
  type SubtitleCandidatePreview,
  type SubtitleSearchCandidate,
  type SubtitleSeasonSearchResponse,
  type SubtitleSeasonTask,
  type SubtitleTrack,
} from '../api/subtitles'
import type { Media } from '../types'
import {
  candidatesForEpisode,
  episodeKey,
  isSubtitleCandidateApplied,
  selectSeasonSubtitleCandidates,
  type SeasonSubtitleStrategy,
} from './librarySeriesSubtitleModel'
import { SubHDCandidateDetails } from './SubHDCandidateDetails'

type Props = { title: string; episodes: Media[] }
type PreviewState = { title: string; content: string; loading: boolean; error: string } | null
type AppliedTracksByMedia = Record<string, SubtitleTrack[]>

const finalStatuses = new Set<SubtitleSeasonTask['status']>(['completed', 'failed'])

export function LibrarySeriesSubtitleSearch({ title, episodes }: Props) {
  const seasons = useMemo(
    () => Array.from(new Set(episodes.map((item) => item.season_num).filter((value) => value > 0))).sort((a, b) => a - b),
    [episodes],
  )
  const [open, setOpen] = useState(false)
  const [season, setSeason] = useState(seasons[0] ?? 1)
  const [searching, setSearching] = useState(false)
  const [applying, setApplying] = useState(false)
  const [singleApplyingMediaID, setSingleApplyingMediaID] = useState('')
  const [error, setError] = useState('')
  const [result, setResult] = useState<SubtitleSeasonSearchResponse | null>(null)
  const [task, setTask] = useState<SubtitleSeasonTask | null>(null)
  const [strategy, setStrategy] = useState<SeasonSubtitleStrategy>('downloads')
  const [selectedCandidateIDs, setSelectedCandidateIDs] = useState<Record<string, string>>({})
  const [openPickerMediaID, setOpenPickerMediaID] = useState('')
  const [preferredUploader, setPreferredUploader] = useState('')
  const [preview, setPreview] = useState<PreviewState>(null)
  const [appliedTracks, setAppliedTracks] = useState<AppliedTracksByMedia>({})

  const seasonEpisodes = useMemo(
    () => episodes
      .filter((item) => item.season_num === season && item.episode_num > 0)
      .sort((a, b) => a.episode_num - b.episode_num || a.id.localeCompare(b.id)),
    [episodes, season],
  )
  const anchor = seasonEpisodes[0]
  const anchorID = anchor?.id || ''
  const activeTaskID = task?.id || ''
  const activeTaskStatus = task?.status
  const active = task != null && !finalStatuses.has(task.status)
  const selectedCount = seasonEpisodes.filter((item) => selectedCandidateIDs[item.id]).length
  const missingCount = seasonEpisodes.length - selectedCount
  const failedDetails = task?.details.filter((item) => item.status === 'failed') ?? []
  const canRetryTask = task != null && finalStatuses.has(task.status) && (failedDetails.length > 0 || task.status === 'failed')
  const activeRetry = active && Boolean(task?.retry_of)

  useEffect(() => {
    if (seasons.length > 0 && !seasons.includes(season)) setSeason(seasons[0])
  }, [season, seasons])

  useEffect(() => {
    setResult(null)
    setTask(null)
    setError('')
    setSelectedCandidateIDs({})
    setOpenPickerMediaID('')
    setPreferredUploader('')
    setAppliedTracks({})
    setSingleApplyingMediaID('')
  }, [season])

  useEffect(() => {
    if (!result) return
    const selection = selectSeasonSubtitleCandidates(seasonEpisodes, result.items, strategy)
    const candidateIDs = { ...selection.candidateIDs }
    for (const episode of seasonEpisodes) {
      const appliedCandidate = candidatesForEpisode(result.items, episode.id)
        .find((candidate) => isSubtitleCandidateApplied(candidate, appliedTracks[episode.id] ?? []))
      if (appliedCandidate) candidateIDs[episode.id] = appliedCandidate.candidate_id
    }
    setSelectedCandidateIDs(candidateIDs)
    setPreferredUploader(selection.uploader)
  }, [appliedTracks, result, seasonEpisodes, strategy])

  useEffect(() => {
    if (!result || active) return
    const mediaIDs = Array.from(new Set(result.items.map((item) => item.media_id).filter(Boolean)))
    let canceled = false
    void Promise.all(mediaIDs.map(async (mediaID) => {
      try {
        return [mediaID, await subtitlesAPI.list(mediaID)] as const
      } catch {
        return [mediaID, []] as const
      }
    }))
      .then((entries) => {
        if (!canceled) setAppliedTracks(Object.fromEntries(entries))
      })
    return () => {
      canceled = true
    }
  }, [active, result])

  useEffect(() => {
    if (!open || !anchorID) return
    const taskID = readTaskID(anchorID, season)
    if (!taskID) return
    let canceled = false
    void subtitlesAPI.getSeasonTask(anchorID, taskID)
      .then((loaded) => {
        if (!canceled) setTask(loaded)
      })
      .catch(() => clearTaskID(anchorID, season))
    return () => {
      canceled = true
    }
  }, [anchorID, open, season])

  useEffect(() => {
    if (!activeTaskID || !anchorID || !activeTaskStatus || finalStatuses.has(activeTaskStatus)) return
    let canceled = false
    const timer = window.setInterval(() => {
      void subtitlesAPI.getSeasonTask(anchorID, activeTaskID)
        .then((loaded) => {
          if (!canceled) setTask(loaded)
        })
        .catch((cause) => {
          if (!canceled) setError(errorMessage(cause, '无法读取整季字幕任务进度'))
        })
    }, 1200)
    return () => {
      canceled = true
      window.clearInterval(timer)
    }
  }, [activeTaskID, activeTaskStatus, anchorID])

  useEffect(() => {
    if (!task?.retry_of || !finalStatuses.has(task.status) || !anchorID) return
    let canceled = false
    void subtitlesAPI.getSeasonTask(anchorID, task.retry_of)
      .then((parent) => {
        if (canceled) return
        setTask(parent)
        saveTaskID(anchorID, season, parent.id)
      })
      .catch((cause) => {
        if (!canceled) setError(errorMessage(cause, '无法读取重试后的整季字幕状态'))
      })
    return () => {
      canceled = true
    }
  }, [anchorID, season, task?.id, task?.retry_of, task?.status])

  const search = async () => {
    if (!anchor || searching || active) return
    setSearching(true)
    setError('')
    setResult(null)
    setTask(null)
    setAppliedTracks({})
    setOpenPickerMediaID('')
    setSingleApplyingMediaID('')
    try {
      const targets = seasonEpisodes.map((episode) => ({
        media_id: episode.id,
        episode_key: episodeKey(season, episode.episode_num),
      }))
      setResult(await subtitlesAPI.searchSeason(anchor.id, season, title, targets, 50))
    } catch (cause) {
      setError(errorMessage(cause, 'SubHD 整季字幕搜索失败'))
    } finally {
      setSearching(false)
    }
  }

  const apply = async () => {
    if (!anchor || !result || applying || active || selectedCount === 0) return
    setApplying(true)
    setError('')
    try {
      const assignments = seasonEpisodes.flatMap((episode) => {
        const candidateID = selectedCandidateIDs[episode.id]
        return candidateID ? [{
          media_id: episode.id,
          episode_key: episodeKey(season, episode.episode_num),
          candidate_id: candidateID,
        }] : []
      })
      const started = await subtitlesAPI.applySeason(anchor.id, result.session_id, season, assignments)
      setTask(started)
      saveTaskID(anchor.id, season, started.id)
    } catch (cause) {
      setError(errorMessage(cause, '无法创建整季字幕应用任务'))
    } finally {
      setApplying(false)
    }
  }

  const applySingle = async (episode: Media, candidate: SubtitleSearchCandidate) => {
    if (!result || applying || active || singleApplyingMediaID || candidate.can_apply === false) return
    setSingleApplyingMediaID(episode.id)
    try {
      const response = await subtitlesAPI.applyCandidate(episode.id, result.session_id, candidate.candidate_id)
      setAppliedTracks((current) => ({ ...current, [episode.id]: response.tracks }))
      setSelectedCandidateIDs((current) => ({ ...current, [episode.id]: candidate.candidate_id }))
      toast.success(`${episodeKey(season, episode.episode_num)} 字幕已应用`)
    } catch (cause) {
      toast.error(errorMessage(cause, `${episodeKey(season, episode.episode_num)} 字幕应用失败`))
    } finally {
      setSingleApplyingMediaID('')
    }
  }

  const retryFailed = async (mediaIDs: string[] = []) => {
    if (!anchor || !task || applying || active || !finalStatuses.has(task.status)) return
    setApplying(true)
    setError('')
    try {
      const started = await subtitlesAPI.retrySeasonTask(anchor.id, task.id, mediaIDs)
      setTask(started)
      saveTaskID(anchor.id, season, started.id)
    } catch (cause) {
      setError(errorMessage(cause, '无法重试失败的字幕任务'))
    } finally {
      setApplying(false)
    }
  }

  const previewCandidate = async (episode: Media, candidate: SubtitleSearchCandidate) => {
    if (!result) return
    setPreview({ title: candidate.filename || candidate.title, content: '', loading: true, error: '' })
    try {
      const loaded: SubtitleCandidatePreview = await subtitlesAPI.previewCandidate(
        episode.id,
        result.session_id,
        candidate.candidate_id,
      )
      setPreview({
        title: loaded.filename || loaded.title,
        content: loaded.content_sample,
        loading: false,
        error: loaded.content_sample ? '' : '字幕源返回了空预览内容',
      })
    } catch (cause) {
      setPreview({
        title: candidate.filename || candidate.title,
        content: '',
        loading: false,
        error: errorMessage(cause, '临时预览失败'),
      })
    }
  }

  if (seasons.length === 0) return null

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className="btn-outline gap-1.5 px-3.5 py-2 text-xs">
        <Captions size={13} className="text-[#c9954a]" />
        <span>SubHD 整季字幕</span>
      </button>
      {open && createPortal(
        <div className="fixed inset-0 z-[120] flex items-end bg-black/45 p-0 backdrop-blur-sm sm:items-center sm:justify-center sm:p-6" role="dialog" aria-modal="true" aria-label="SubHD 整季字幕">
          <div className="flex max-h-[94vh] w-full max-w-5xl flex-col overflow-hidden rounded-t-3xl bg-white shadow-2xl sm:max-h-[88vh] sm:rounded-3xl">
            <div className="flex shrink-0 items-start justify-between gap-4 border-b border-sand-200 px-5 py-4 sm:px-7">
              <div className="min-w-0">
                <p className="text-xs font-bold uppercase tracking-[0.16em] text-[#c9954a]">手动整季字幕</p>
                <h3 className="mt-1 truncate text-lg font-bold text-ink-600">{title}</h3>
              </div>
              <button type="button" onClick={() => setOpen(false)} className="btn-ghost h-9 w-9 shrink-0 justify-center p-0" aria-label="关闭">
                <X size={18} />
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4 sm:px-7">
              <div className="flex flex-wrap items-center gap-2">
                <select aria-label="字幕季" value={season} onChange={(event) => setSeason(Number(event.target.value))} disabled={searching || active} className="h-10 rounded-lg border border-sand-300 bg-white px-3 text-sm font-semibold text-ink-600">
                  {seasons.map((value) => (
                    <option key={value} value={value}>第 {value} 季 · {episodes.filter((item) => item.season_num === value).length} 个片源</option>
                  ))}
                </select>
                <button type="button" onClick={() => void search()} disabled={searching || active || seasonEpisodes.length === 0} className="btn-primary h-10 gap-2 px-4">
                  {searching ? <LoaderCircle size={16} className="animate-spin" /> : <Search size={16} />}
                  {searching ? `正在查询第 ${season} 季` : '查询'}
                </button>
                {result && (
                  <select aria-label="自动选择方式" value={strategy} onChange={(event) => setStrategy(event.target.value as SeasonSubtitleStrategy)} disabled={active || applying} className="h-10 rounded-lg border border-sand-300 bg-white px-3 text-sm font-semibold text-ink-600">
                    <option value="downloads">最高下载量（默认）</option>
                    <option value="uploader">同一上传人优先</option>
                  </select>
                )}
              </div>

              <div className="mt-4 rounded-2xl border border-sand-200 bg-sand-50/70 p-4 text-sm text-ink-50" aria-live="polite">
                {searching ? (
                  <div className="flex items-center gap-3">
                    <LoaderCircle size={18} className="animate-spin text-[#c9954a]" />
                    <span>正在查找与“{title} S{String(season).padStart(2, '0')}”匹配的 SubHD 作品详情页并读取逐集候选；此步骤不会保存字幕。</span>
                  </div>
                ) : task ? (
                  <TaskProgress
                    task={task}
                    retrying={applying}
                    onRetryEpisode={(mediaID) => void retryFailed([mediaID])}
                  />
                ) : result ? (
                  <div className="space-y-1">
                    <p>已为 {selectedCount}/{seasonEpisodes.length} 个片源选中字幕{missingCount > 0 ? `，${missingCount} 个片源没有逐集候选` : ''}。</p>
                    {strategy === 'uploader' && (
                      <p>{preferredUploader ? `优先上传人：${preferredUploader}；缺少该上传人的集数自动使用本集最高下载量。` : '没有覆盖至少两集的统一上传人，已全部使用各集最高下载量。'}</p>
                    )}
                    {result.detail_url && (
                      <a href={result.detail_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-[#9b6a2f] hover:underline">
                        {result.detail_title || '打开 SubHD 作品详情页'} <ExternalLink size={12} />
                      </a>
                    )}
                  </div>
                ) : <span>只在此处手动查询和确认；不会在入库、扫描或定时任务中自动匹配。</span>}
                {error && <div className="mt-3 rounded-lg bg-red-50 p-3 text-red-600">{error}</div>}
              </div>

              {result && !active && (
                <div className="mt-4 space-y-3">
                  {seasonEpisodes.map((episode) => {
                    const candidates = candidatesForEpisode(result.items, episode.id)
                    const selectedID = selectedCandidateIDs[episode.id] || ''
                    const selected = candidates.find((candidate) => candidate.candidate_id === selectedID)
                    return (
                      <EpisodeSubtitleCard
                        key={episode.id}
                        episode={episode}
                        season={season}
                        candidates={candidates}
                        tracks={appliedTracks[episode.id] ?? []}
                        selected={selected}
                        selectedID={selectedID}
                        pickerOpen={openPickerMediaID === episode.id}
                        singleApplying={singleApplyingMediaID === episode.id}
                        disabled={applying || Boolean(singleApplyingMediaID)}
                        onTogglePicker={() => setOpenPickerMediaID((current) => current === episode.id ? '' : episode.id)}
                        onSelect={(candidateID) => {
                          setSelectedCandidateIDs((current) => ({ ...current, [episode.id]: candidateID }))
                          setOpenPickerMediaID('')
                        }}
                        onPreview={(candidate) => void previewCandidate(episode, candidate)}
                        onApply={(candidate) => void applySingle(episode, candidate)}
                      />
                    )
                  })}
                </div>
              )}
            </div>

            <div className="flex shrink-0 items-center justify-between gap-4 border-t border-sand-200 bg-white px-5 py-4 sm:px-7">
              <p className="text-xs text-sand-500">{canRetryTask || activeRetry ? '重试只处理失败或未完成项，已成功的集数不会重复应用。' : '应用前可逐集切换候选并临时预览；只有右侧按钮会写入字幕。'}</p>
              {canRetryTask || activeRetry ? (
                <button type="button" onClick={() => void retryFailed()} disabled={active || applying} className="btn-primary min-w-32 shrink-0 gap-2">
                  {applying || active ? <LoaderCircle size={15} className="animate-spin" /> : <RotateCcw size={15} />}
                  {active ? '重试中' : failedDetails.length > 0 ? `重试失败项（${failedDetails.length}）` : '重试未完成项'}
                </button>
              ) : (
                <button type="button" onClick={() => void apply()} disabled={!result || active || applying || selectedCount === 0} className="btn-primary min-w-28 shrink-0 gap-2">
                  {applying || active ? <LoaderCircle size={15} className="animate-spin" /> : <Check size={15} />}
                  {active ? '应用中' : `应用${selectedCount > 0 ? `（${selectedCount}）` : ''}`}
                </button>
              )}
            </div>
          </div>
        </div>,
        document.body,
      )}
      {preview && createPortal(
        <SubtitlePreviewDialog preview={preview} onClose={() => setPreview(null)} />,
        document.body,
      )}
    </>
  )
}

function EpisodeSubtitleCard({
  episode,
  season,
  candidates,
  tracks,
  selected,
  selectedID,
  pickerOpen,
  singleApplying,
  disabled,
  onTogglePicker,
  onSelect,
  onPreview,
  onApply,
}: {
  episode: Media
  season: number
  candidates: SubtitleSearchCandidate[]
  tracks: SubtitleTrack[]
  selected?: SubtitleSearchCandidate
  selectedID: string
  pickerOpen: boolean
  singleApplying: boolean
  disabled: boolean
  onTogglePicker: () => void
  onSelect: (candidateID: string) => void
  onPreview: (candidate: SubtitleSearchCandidate) => void
  onApply: (candidate: SubtitleSearchCandidate) => void
}) {
  const label = episodeKey(season, episode.episode_num)
  const detailCandidate = selected ?? candidates[0]
  const detailApplied = Boolean(detailCandidate && isSubtitleCandidateApplied(detailCandidate, tracks))
  return (
    <section className="rounded-2xl border border-sand-200 bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-md bg-[#fff4e4] px-2 py-1 text-xs font-bold text-[#a96d27]">{label}</span>
            <h4 className="min-w-0 truncate text-sm font-semibold text-ink-600">{episode.episode_title || episode.title}</h4>
            {candidates.length > 0 && !selected && <span className="rounded bg-amber-50 px-2 py-1 text-xs font-medium text-amber-700">候选均为当前不可应用格式</span>}
          </div>
          {candidates.length > 0 ? (
            <div className="mt-3 flex items-center gap-2">
              <div className="relative min-w-0 flex-1">
                <button
                  type="button"
                  aria-haspopup="listbox"
                  aria-expanded={pickerOpen}
                  onClick={onTogglePicker}
                  disabled={disabled || candidates.every((candidate) => candidate.can_apply === false)}
                  className="flex h-10 w-full items-center gap-2 rounded-lg border border-sand-300 bg-white px-3 text-left text-sm text-ink-600 outline-none focus-visible:border-[#d39b41] focus-visible:ring-0 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <span className="min-w-0 flex-1 truncate">{selected ? subtitleCandidatePickerLabel(selected) : '请选择字幕'}</span>
                  <ChevronDown size={16} className={`shrink-0 text-sand-500 transition-transform ${pickerOpen ? 'rotate-180' : ''}`} />
                </button>
                {pickerOpen && (
                  <div role="listbox" className="absolute z-20 mt-1 max-h-72 w-full overflow-y-auto rounded-lg border border-sand-300 bg-white py-1 shadow-lg">
                    {candidates.map((candidate) => {
                      const applied = isSubtitleCandidateApplied(candidate, tracks)
                      const unavailable = candidate.can_apply === false
                      return (
                        <button
                          key={candidate.candidate_id}
                          type="button"
                          role="option"
                          aria-selected={candidate.candidate_id === selectedID}
                          disabled={unavailable || disabled}
                          onClick={() => {
                            onSelect(candidate.candidate_id)
                          }}
                          className="flex w-full items-baseline gap-2 px-3 py-2 text-left text-sm text-ink-600 hover:bg-sand-50 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          {applied ? <Check size={16} className="relative top-1 shrink-0 text-emerald-600" aria-label="当前已应用" /> : <span className="h-4 w-4 shrink-0" aria-hidden="true" />}
                          <span className="min-w-0 flex-1 truncate">{subtitleCandidatePickerLabel(candidate)}{unavailable ? ` · ${candidate.unavailable_reason || '当前不可应用'}` : ''}</span>
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
              <div className="flex shrink-0 gap-2">
                <button type="button" onClick={() => selected && onPreview(selected)} disabled={!selected || selected.can_preview === false || disabled} className="btn-outline h-10 gap-1.5 px-3 text-xs">
                  <Eye size={14} /> 预览
                </button>
                <button type="button" onClick={() => selected && onApply(selected)} disabled={!selected || selected.can_apply === false || disabled} className="btn-primary h-10 gap-1.5 px-3 text-xs">
                  {singleApplying ? <LoaderCircle size={14} className="animate-spin" /> : <Check size={14} />}
                  {singleApplying ? '应用中' : '应用本集'}
                </button>
              </div>
            </div>
          ) : <p className="mt-3 rounded-lg bg-sand-50 px-3 py-2 text-sm text-sand-500">SubHD 详情页没有这一集的可用字幕。</p>}
        </div>
      </div>
      {detailCandidate && <SubHDCandidateDetails candidate={detailCandidate} applied={detailApplied} />}
    </section>
  )
}

function subtitleCandidatePickerLabel(candidate: SubtitleSearchCandidate): string {
  const formats = Array.isArray(candidate.formats) ? candidate.formats.filter(Boolean) : []
  const format = formats.join('/') || candidate.filename.split('.').pop()?.toUpperCase() || '字幕'
  return `${candidate.download_count.toLocaleString()} 下载 · ${candidate.uploader || '未知上传人'} · ${format}`
}

function TaskProgress({
  task,
  retrying,
  onRetryEpisode,
}: {
  task: SubtitleSeasonTask
  retrying: boolean
  onRetryEpisode: (mediaID: string) => void
}) {
  const operation = task.retry_of ? '重试' : '应用'
  const stage = task.stage === 'download'
    ? `正在下载并${operation}当前集字幕`
    : task.stage === 'applying'
      ? `正在${operation}当前集字幕缓存`
      : task.status === 'queued'
        ? '任务已排队'
        : task.status === 'completed'
          ? task.failed > 0 ? '整季字幕应用完成，部分集失败' : '整季字幕应用完成'
          : '整季字幕应用失败'
  const failures = task.details.filter((item) => item.status === 'failed' || (item.status === 'skipped' && item.error))
  return (
    <div>
      <p className="font-semibold text-ink-600">{stage}</p>
      <p className="mt-1">进度 {task.progress_current}/{task.progress_total} · 成功 {task.succeeded} · 跳过 {task.skipped} · 失败 {task.failed}{task.current_episode ? ` · 当前 ${task.current_episode}` : ''}</p>
      {task.error && <p className="mt-2 rounded-lg bg-red-50 p-2 text-red-600">{task.error}</p>}
      {failures.length > 0 && (
        <div className="mt-2 space-y-2 text-xs text-sand-600">
          {failures.map((item) => (
            <div key={item.media_id} className="flex items-start justify-between gap-3 rounded-lg bg-white/70 px-3 py-2">
              <p className="min-w-0 break-words">{item.episode_key}：{item.status === 'failed' ? '失败' : '跳过'}{item.error ? ` · ${item.error}` : ''}</p>
              {item.status === 'failed' && finalStatuses.has(task.status) && (
                <button
                  type="button"
                  onClick={() => onRetryEpisode(item.media_id)}
                  disabled={retrying}
                  className="btn-outline h-8 shrink-0 gap-1.5 px-2.5 text-xs"
                  title={`仅重试 ${item.episode_key}`}
                >
                  {retrying ? <LoaderCircle size={13} className="animate-spin" /> : <RotateCcw size={13} />}
                  重试
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      {!finalStatuses.has(task.status) && <p className="mt-2 text-xs text-sand-500">可关闭详情页；再次打开本季字幕面板会恢复此任务进度。</p>}
    </div>
  )
}

function SubtitlePreviewDialog({ preview, onClose }: { preview: NonNullable<PreviewState>; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-[130] flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label="字幕预览">
      <div className="flex max-h-[min(78vh,680px)] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex shrink-0 items-center gap-3 border-b border-gray-200 px-5 py-4">
          <div className="min-w-0 flex-1">
            <h3 className="font-semibold text-ink-700">字幕预览</h3>
            <p className="mt-1 truncate text-xs text-sand-500" title={preview.title}>{preview.title}</p>
          </div>
          <button type="button" onClick={onClose} className="btn-ghost h-9 w-9 justify-center p-0" aria-label="关闭字幕预览"><X size={17} /></button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto bg-gray-950 p-5">
          {preview.loading && <div className="flex items-center justify-center gap-2 py-20 text-sm text-gray-300"><LoaderCircle size={17} className="animate-spin" />正在加载预览</div>}
          {!preview.loading && preview.error && <p className="rounded-lg bg-red-950/60 px-4 py-3 text-sm text-red-200">{preview.error}</p>}
          {!preview.loading && preview.content && <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-7 text-gray-100">{preview.content}</pre>}
        </div>
      </div>
    </div>
  )
}

function errorMessage(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error
    || (error instanceof Error ? error.message : fallback)
}

function taskKey(mediaID: string, season: number) {
  return `mediastation:season-subtitle-task:${mediaID}:${season}`
}

function readTaskID(mediaID: string, season: number) {
  try { return window.localStorage.getItem(taskKey(mediaID, season)) } catch { return null }
}

function saveTaskID(mediaID: string, season: number, taskID: string) {
  try { window.localStorage.setItem(taskKey(mediaID, season), taskID) } catch { /* task remains persisted on the server */ }
}

function clearTaskID(mediaID: string, season: number) {
  try { window.localStorage.removeItem(taskKey(mediaID, season)) } catch { /* no-op */ }
}
