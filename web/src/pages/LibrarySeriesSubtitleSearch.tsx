import { useEffect, useMemo, useState } from 'react'
import { Captions, Check, LoaderCircle, Search, X } from 'lucide-react'

import { subtitlesAPI, type SubtitleSeasonSearchResponse, type SubtitleSeasonTask } from '../api/subtitles'
import type { Media } from '../types'

type Props = { title: string; episodes: Media[] }

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
  const [error, setError] = useState('')
  const [result, setResult] = useState<SubtitleSeasonSearchResponse | null>(null)
  const [task, setTask] = useState<SubtitleSeasonTask | null>(null)
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())

  const seasonEpisodes = useMemo(
    () => episodes.filter((item) => item.season_num === season && item.episode_num > 0).sort((a, b) => a.episode_num - b.episode_num),
    [episodes, season],
  )
  const anchor = seasonEpisodes[0]
  const active = task != null && !finalStatuses.has(task.status)
  const selectedCount = seasonEpisodes.filter((item) => selectedIDs.has(item.id)).length

  useEffect(() => {
    if (seasons.length > 0 && !seasons.includes(season)) setSeason(seasons[0])
  }, [season, seasons])

  useEffect(() => {
    setSelectedIDs(new Set(seasonEpisodes.map((item) => item.id)))
  }, [seasonEpisodes])

  useEffect(() => {
    if (!open || !anchor) return
    const taskID = readTaskID(anchor.id, season)
    if (!taskID) return
    let canceled = false
    void subtitlesAPI.getSeasonTask(anchor.id, taskID)
      .then((loaded) => {
        if (!canceled) setTask(loaded)
      })
      .catch(() => clearTaskID(anchor.id, season))
    return () => {
      canceled = true
    }
  }, [anchor?.id, open, season])

  useEffect(() => {
    if (!task || !anchor || finalStatuses.has(task.status)) return
    let canceled = false
    const timer = window.setInterval(() => {
      void subtitlesAPI.getSeasonTask(anchor.id, task.id)
        .then((loaded) => {
          if (!canceled) setTask(loaded)
        })
        .catch((cause) => {
          if (!canceled) setError(cause instanceof Error ? cause.message : '\u65e0\u6cd5\u8bfb\u53d6\u6574\u5b63\u5b57\u5e55\u4efb\u52a1\u8fdb\u5ea6')
        })
    }, 1200)
    return () => {
      canceled = true
      window.clearInterval(timer)
    }
  }, [anchor?.id, task?.id, task?.status])

  const search = async () => {
    if (!anchor || searching || active) return
    setSearching(true)
    setError('')
    setResult(null)
    try {
      setResult(await subtitlesAPI.searchSeason(anchor.id, season, title))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'SubHD \u6574\u5b63\u5b57\u5e55\u641c\u7d22\u5931\u8d25')
    } finally {
      setSearching(false)
    }
  }

  const apply = async (candidateID: string) => {
    if (!anchor || !result || applying || active || selectedCount === 0) return
    setApplying(true)
    setError('')
    try {
      const started = await subtitlesAPI.applySeason(
        anchor.id,
        result.session_id,
        candidateID,
        season,
        seasonEpisodes.filter((item) => selectedIDs.has(item.id)).map((item) => item.id),
      )
      setTask(started)
      saveTaskID(anchor.id, season, started.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '\u65e0\u6cd5\u521b\u5efa\u6574\u5b63\u5b57\u5e55\u5e94\u7528\u4efb\u52a1')
    } finally {
      setApplying(false)
    }
  }

  const toggle = (id: string) => {
    if (active || applying) return
    setSelectedIDs((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  if (seasons.length === 0) return null

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
        <Captions size={13} className="text-[#c9954a]" />
        <span>{'SubHD \u6574\u5b63\u5b57\u5e55'}</span>
      </button>
      {open && (
        <div className="fixed inset-0 z-[70] flex items-end bg-black/45 p-0 sm:items-center sm:justify-center sm:p-6" role="dialog" aria-modal="true" aria-label={'SubHD \u6574\u5b63\u5b57\u5e55'}>
          <div className="w-full max-w-3xl rounded-t-3xl bg-white p-5 shadow-2xl sm:rounded-3xl sm:p-7">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-xs font-bold uppercase tracking-[0.16em] text-[#c9954a]">{'\u624b\u52a8\u6574\u5b63\u5b57\u5e55'}</p>
                <h3 className="mt-1 text-lg font-bold text-ink-600">{title}</h3>
              </div>
              <button type="button" onClick={() => setOpen(false)} className="btn-ghost h-9 w-9 justify-center p-0" aria-label={'\u5173\u95ed'}>
                <X size={18} />
              </button>
            </div>

            <div className="mt-5 flex flex-wrap items-end gap-3">
              <label className="flex flex-col gap-1 text-xs font-semibold text-sand-600">
                {'\u5b63'}
                <select value={season} onChange={(event) => setSeason(Number(event.target.value))} disabled={searching || active} className="rounded-lg border border-sand-300 bg-white px-3 py-2 text-sm text-ink-600">
                  {seasons.map((value) => <option key={value} value={value}>{'\u7b2c ' + value + ' \u5b63 \u00b7 ' + episodes.filter((item) => item.season_num === value).length + ' \u96c6'}</option>)}
                </select>
              </label>
              <button type="button" onClick={() => void search()} disabled={searching || active || seasonEpisodes.length === 0} className="btn-primary gap-2">
                {searching ? <LoaderCircle size={16} className="animate-spin" /> : <Search size={16} />}
                {searching ? 'SubHD \u6b63\u5728\u641c\u7d22\u7b2c ' + season + ' \u5b63' : '\u641c\u7d22\u6574\u5b63\u5019\u9009'}
              </button>
            </div>

            <div className="mt-4 rounded-2xl border border-sand-200 bg-sand-50/70 p-4 text-sm text-ink-50" aria-live="polite">
              {searching ? (
                <div className="flex items-center gap-3">
                  <LoaderCircle size={18} className="animate-spin text-[#c9954a]" />
                  <span>{'SubHD \u641c\u7d22\u4e2d\uff1a\u4ee5\u201c' + title + ' S' + String(season).padStart(2, '0') + '\u201d\u67e5\u8be2\uff0c\u76ee\u6807 ' + seasonEpisodes.length + ' \u96c6\uff1b\u6b64\u6b65\u9aa4\u4e0d\u4e0b\u8f7d\u3001\u4e0d\u5199\u5165\u5b57\u5e55\u7f13\u5b58\u3002'}</span>
                </div>
              ) : task ? <TaskProgress task={task} /> : result ? (
                <span>{'SubHD \u8fd4\u56de ' + result.items.length + ' \u4e2a\u5019\u9009\u3002\u9009\u62e9\u76ee\u6807\u96c6\u548c\u4e00\u4e2a\u5019\u9009\u540e\uff0c\u7cfb\u7edf\u624d\u4f1a\u521b\u5efa\u5e94\u7528\u4efb\u52a1\u3002'}</span>
              ) : <span>{'\u53ea\u7531\u7ba1\u7406\u5458\u5728\u6b64\u5904\u624b\u52a8\u53d1\u8d77\uff1b\u4e0d\u4f1a\u5728\u5165\u5e93\u3001\u626b\u63cf\u6216\u540e\u53f0\u5b9a\u65f6\u4efb\u52a1\u4e2d\u81ea\u52a8\u641c\u7d22\u6216\u5e94\u7528\u3002'}</span>}
              {error && <div className="mt-3 rounded-lg bg-red-50 p-3 text-red-600">{error}</div>}
            </div>

            {!active && (
              <div className="mt-4 rounded-xl border border-sand-200 p-3">
                <div className="flex items-center justify-between gap-3 text-sm font-semibold text-ink-600">
                  <span>{'\u76ee\u6807\u96c6\uff1a\u5df2\u9009 ' + selectedCount + '/' + seasonEpisodes.length}</span>
                  <button type="button" className="text-xs text-[#9b6a2f]" onClick={() => setSelectedIDs(new Set(seasonEpisodes.map((item) => item.id)))}>{'\u5168\u9009\u672c\u5b63'}</button>
                </div>
                <div className="mt-2 max-h-32 space-y-1 overflow-y-auto pr-1">
                  {seasonEpisodes.map((episode) => (
                    <label key={episode.id} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs text-ink-50 hover:bg-sand-50">
                      <input type="checkbox" checked={selectedIDs.has(episode.id)} onChange={() => toggle(episode.id)} disabled={applying} />
                      <span>{'S' + String(season).padStart(2, '0') + 'E' + String(episode.episode_num).padStart(2, '0') + ' \u00b7 ' + (episode.episode_title || episode.title)}</span>
                    </label>
                  ))}
                </div>
              </div>
            )}

            {result && !active && (
              <div className="mt-4 max-h-[34vh] space-y-2 overflow-y-auto pr-1">
                {result.items.map((candidate) => (
                  <div key={candidate.candidate_id} className="rounded-xl border border-sand-200 bg-white p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-sand-500"><span className="font-semibold text-[#c9954a]">SubHD</span><span>{candidate.language || '\u8bed\u8a00\u672a\u6807\u6ce8'}</span></div>
                        <p className="mt-1 break-words text-sm font-medium text-ink-600">{candidate.title || candidate.filename || '\u672a\u547d\u540d\u5b57\u5e55'}</p>
                      </div>
                      <button type="button" disabled={applying || selectedCount === 0} onClick={() => void apply(candidate.candidate_id)} className="btn-outline shrink-0 px-3 py-1.5 text-xs">
                        {applying ? <LoaderCircle size={14} className="animate-spin" /> : <Check size={14} />}
                        {'\u5e94\u7528\u5230\u5df2\u9009 ' + selectedCount + ' \u96c6'}
                      </button>
                    </div>
                  </div>
                ))}
                {result.items.length === 0 && <p className="py-6 text-center text-sm text-sand-500">{'SubHD \u672a\u8fd4\u56de\u53ef\u7528\u5019\u9009\u3002'}</p>}
              </div>
            )}
          </div>
        </div>
      )}
    </>
  )
}

function TaskProgress({ task }: { task: SubtitleSeasonTask }) {
  const stage = task.stage === 'download'
    ? '\u6b63\u5728\u4e0b\u8f7d\u6574\u5b63\u5305\uff08\u53ea\u4e0b\u8f7d\u4e00\u6b21\uff09'
    : task.stage === 'applying'
      ? '\u6b63\u5728\u6309\u96c6\u5199\u5165\u5b57\u5e55\u7f13\u5b58'
      : task.status === 'queued'
        ? '\u4efb\u52a1\u5df2\u6392\u961f'
        : task.status === 'completed'
          ? '\u6574\u5b63\u5b57\u5e55\u5e94\u7528\u5b8c\u6210'
          : '\u6574\u5b63\u5b57\u5e55\u5e94\u7528\u5931\u8d25'
  const failures = task.details.filter((item) => item.status === 'failed' || (item.status === 'skipped' && item.error)).slice(-6)
  return (
    <div>
      <p className="font-semibold text-ink-600">{stage}</p>
      <p className="mt-1">{'\u8fdb\u5ea6 ' + task.progress_current + '/' + task.progress_total + ' \u00b7 \u6210\u529f ' + task.succeeded + ' \u00b7 \u8df3\u8fc7 ' + task.skipped + ' \u00b7 \u5931\u8d25 ' + task.failed + (task.current_episode ? ' \u00b7 \u5f53\u524d ' + task.current_episode : '')}</p>
      {task.error && <p className="mt-2 rounded-lg bg-red-50 p-2 text-red-600">{task.error}</p>}
      {failures.length > 0 && <div className="mt-2 space-y-1 text-xs text-sand-600">{failures.map((item) => <p key={item.media_id}>{item.episode_key + '\uff1a' + (item.status === 'failed' ? '\u5931\u8d25' : '\u8df3\u8fc7') + (item.error ? ' \u00b7 ' + item.error : '')}</p>)}</div>}
      {!finalStatuses.has(task.status) && <p className="mt-2 text-xs text-sand-500">{'\u53ef\u5173\u95ed\u8be6\u60c5\u9875\uff1b\u518d\u6b21\u6253\u5f00\u672c\u5b63\u5b57\u5e55\u9762\u677f\u4f1a\u6062\u590d\u6b64\u4efb\u52a1\u8fdb\u5ea6\u3002'}</p>}
    </div>
  )
}

function taskKey(mediaID: string, season: number) {
  return 'mediastation:season-subtitle-task:' + mediaID + ':' + season
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
