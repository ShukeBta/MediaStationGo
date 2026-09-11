import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import toast from 'react-hot-toast'
import {
  Download,
  FileUp,
  LoaderCircle,
  MessageSquare,
  Minus,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react'

import {
  danmakuAPI,
  describeDanmakuError,
  type DanmakuMatchResult,
  type DanmakuPrewarmTask,
  type DanmakuSearchResult,
} from '../api/danmaku'
import { confirmAction } from '../components/confirmAction'
import { useAuthStore } from '../stores/auth'
import type { MediaVersion } from '../types'
import { mediaFilename } from '../utils/mediaFilename'
import {
  DANMAKU_IMPORT_MAX_BYTES,
  describeImportSummary,
  importFormatFromFilename,
} from './mediaDetailDanmakuModel'

// 媒体详情页的弹幕区块：展示「这一集挂到了哪个弹幕库」，并提供人工修正入口。
// 读取对所有能播放的用户开放；改写关联（匹配/偏移/清除）是管理员操作。

type MediaDetailDanmakuProps = {
  mediaId: string
  versions: MediaVersion[]
  versionsLoading: boolean
}

const OFFSET_STEPS = [-1, -0.5, 0.5, 1]

export function MediaDetailDanmaku({ mediaId, versions, versionsLoading }: MediaDetailDanmakuProps) {
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const defaultMediaId = useMemo(
    () => versions.find((version) => version.is_current)?.id || mediaId,
    [mediaId, versions],
  )
  const [selectedMediaId, setSelectedMediaId] = useState(defaultMediaId)
  const [state, setState] = useState<DanmakuMatchResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [busy, setBusy] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [prewarm, setPrewarm] = useState<DanmakuPrewarmTask | null>(null)
  const importInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    setSelectedMediaId(defaultMediaId)
  }, [defaultMediaId])

  const loadState = useCallback(async () => {
    if (!selectedMediaId) return
    setLoading(true)
    setLoadError('')
    try {
      setState(await danmakuAPI.state(selectedMediaId))
    } catch (error) {
      setState(null)
      setLoadError(describeDanmakuError(error).message)
    } finally {
      setLoading(false)
    }
  }, [selectedMediaId])

  useEffect(() => {
    void loadState()
  }, [loadState])

  const runAutoMatch = useCallback(async () => {
    setBusy('match')
    try {
      const result = await danmakuAPI.match(selectedMediaId)
      setState(result)
      if (result.matched) {
        toast.success(`已匹配：${[result.anime_title, result.episode_title].filter(Boolean).join(' ')}`)
      } else {
        // 未匹配不是异常，但必须说清楚"试过什么"，否则用户只能反复点。
        toast.error('没有匹配到弹幕，可尝试手动搜索')
      }
    } catch (error) {
      toast.error(describeDanmakuError(error).message)
    } finally {
      setBusy('')
    }
  }, [selectedMediaId])

  const applyCandidate = useCallback(
    async (episodeId: string, animeTitle: string, episodeTitle: string) => {
      setBusy('apply')
      try {
        await danmakuAPI.setManual(selectedMediaId, {
          episode_id: episodeId,
          anime_title: animeTitle,
          episode_title: episodeTitle,
          offset_seconds: state?.shift ?? 0,
        })
        setSearchOpen(false)
        toast.success('已保存手动匹配')
        await loadState()
      } catch (error) {
        toast.error(describeDanmakuError(error).message)
      } finally {
        setBusy('')
      }
    },
    [loadState, selectedMediaId, state],
  )

  const adjustOffset = useCallback(
    async (delta: number) => {
      setBusy('offset')
      try {
        const current = state ? Number(state.shift || 0) : 0
        const next = Math.round((current + delta) * 10) / 10
        if (next < -600 || next > 600) {
          toast.error('时间轴偏移需在 ±600 秒以内')
          return
        }
        await danmakuAPI.setOffset(selectedMediaId, next)
        await loadState()
      } catch (error) {
        toast.error(describeDanmakuError(error).message)
      } finally {
        setBusy('')
      }
    },
    [loadState, selectedMediaId, state],
  )

  const resetOffset = useCallback(async () => {
    setBusy('offset')
    try {
      await danmakuAPI.setOffset(selectedMediaId, 0)
      await loadState()
    } catch (error) {
      toast.error(describeDanmakuError(error).message)
    } finally {
      setBusy('')
    }
  }, [loadState, selectedMediaId])

  // 整季预热：后台逐集跑，这里只轮询进度；失败/空结果都在任务详情里如实展示。
  const pollPrewarm = useCallback(
    async (taskId: string) => {
      try {
        const task = await danmakuAPI.prewarmTask(selectedMediaId, taskId)
        setPrewarm(task)
        if (task.status === 'queued' || task.status === 'running') {
          window.setTimeout(() => void pollPrewarm(taskId), 2000)
          return
        }
        if (task.status === 'completed') {
          toast.success(
            `预热完成：${task.matched} 集有弹幕，${task.empty} 集为空${task.failed ? `，${task.failed} 集失败` : ''}`,
          )
        } else {
          toast.error(task.error || '预热中断')
        }
      } catch (error) {
        toast.error(describeDanmakuError(error).message)
      }
    },
    [selectedMediaId],
  )

  const startPrewarm = useCallback(async () => {
    setBusy('prewarm')
    try {
      const task = await danmakuAPI.prewarmSeason(selectedMediaId)
      setPrewarm(task)
      void pollPrewarm(task.task_id)
    } catch (error) {
      toast.error(describeDanmakuError(error).message)
    } finally {
      setBusy('')
    }
  }, [pollPrewarm, selectedMediaId])

  const clearMatch = useCallback(async () => {    const confirmed = await confirmAction({
      title: '清除弹幕关联',
      message: '确定清除这一集的弹幕匹配吗？清除后下次播放会重新自动匹配。',
      confirmText: '清除关联',
    })
    if (!confirmed) return
    setBusy('clear')
    try {
      await danmakuAPI.clear(selectedMediaId)
      toast.success('已清除弹幕关联')
      await loadState()
    } catch (error) {
      toast.error(describeDanmakuError(error).message)
    } finally {
      setBusy('')
    }
  }, [loadState, selectedMediaId])

  // 导入用户手里的弹幕文件：官方库匹配不到（自制、冷门、非番剧）时的兜底手段。
  // 解析与归一化都在服务端，这里只负责读文件、按扩展名给个格式提示，并如实展示统计。
  const importLocalFile = useCallback(
    async (file: File) => {
      if (file.size > DANMAKU_IMPORT_MAX_BYTES) {
        toast.error(`文件 ${(file.size / 1024 / 1024).toFixed(1)}MB 超过 8MB 上限`)
        return
      }
      setBusy('import')
      try {
        const content = await file.text()
        const { imported } = await danmakuAPI.importLocal(selectedMediaId, {
          content,
          format: importFormatFromFilename(file.name),
        })
        toast.success(describeImportSummary(imported))
        await loadState()
      } catch (error) {
        toast.error(describeDanmakuError(error).message)
      } finally {
        setBusy('')
      }
    },
    [loadState, selectedMediaId],
  )

  const status = state?.status || (loading ? 'loading' : 'unmatched')
  const matched = Boolean(state?.matched)
  const isLocalImport = state?.source === 'local'
  const statusText = loading
    ? '检查中'
    : status === 'matched'
      ? [
          isLocalImport ? `本地导入${state?.local_format ? `（${state.local_format}）` : ''}` : state?.source,
          state?.anime_title,
          state?.episode_title,
        ]
          .filter(Boolean)
          .join(' · ')
      : status === 'failed'
        ? '回源失败'
        : '未匹配'

  return (
    <section className="space-y-3" aria-label="弹幕">
      <div className="flex flex-wrap items-center gap-2">
        <MessageSquare size={16} className="text-[#c9954a]" />
        <h2 className="text-sm font-semibold text-ink-600">弹幕</h2>
        <span className="text-xs text-sand-500">{statusText}</span>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {versions.length > 1 && (
            <select
              value={selectedMediaId}
              disabled={versionsLoading}
              onChange={(event) => setSelectedMediaId(event.target.value)}
              className="h-9 max-w-60 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
              aria-label="选择弹幕所属片源版本"
            >
              {versions.map((version) => (
                <option key={version.id} value={version.id}>
                  {`${version.is_current ? '当前 · ' : ''}${mediaFilename(version)}`}
                </option>
              ))}
            </select>
          )}
          <button
            type="button"
            onClick={() => void loadState()}
            disabled={loading}
            className="btn-outline h-9 w-9 justify-center p-0"
            title="刷新弹幕状态"
            aria-label="刷新弹幕状态"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          {isAdmin && (
            <>
              <button
                type="button"
                onClick={() => void runAutoMatch()}
                disabled={busy === 'match'}
                className="btn-outline h-9 gap-1.5 px-3 text-xs"
              >
                {busy === 'match' ? <LoaderCircle size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                自动匹配
              </button>
              <button
                type="button"
                onClick={() => setSearchOpen(true)}
                className="btn-outline h-9 gap-1.5 px-3 text-xs"
              >
                <Search size={14} />
                搜索弹幕
              </button>
              <input
                ref={importInputRef}
                type="file"
                accept=".xml,.json,application/xml,application/json"
                className="hidden"
                aria-label="导入本地弹幕文件"
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  // 同一个文件连续选两次也要能触发 change。
                  event.target.value = ''
                  if (file) void importLocalFile(file)
                }}
              />
              <button
                type="button"
                onClick={() => importInputRef.current?.click()}
                disabled={busy === 'import'}
                className="btn-outline h-9 gap-1.5 px-3 text-xs"
                title="导入本地的 B 站 XML 或弹弹play JSON 弹幕文件（匹配不到时用）"
              >
                {busy === 'import' ? <LoaderCircle size={14} className="animate-spin" /> : <FileUp size={14} />}
                导入文件
              </button>
              <button
                type="button"
                onClick={() => void startPrewarm()}
                disabled={busy === 'prewarm' || prewarm?.status === 'running' || prewarm?.status === 'queued'}
                className="btn-outline h-9 gap-1.5 px-3 text-xs"
                title="逐集预热整季弹幕（串行、带延迟），打开剧集时无需再等回源"
              >
                {busy === 'prewarm' || prewarm?.status === 'running' ? (
                  <LoaderCircle size={14} className="animate-spin" />
                ) : (
                  <Download size={14} />
                )}
                预热整季
              </button>
              {matched && (
                <button
                  type="button"
                  onClick={() => void clearMatch()}
                  disabled={busy === 'clear'}
                  className="btn-ghost h-9 w-9 justify-center p-0"
                  title="清除弹幕关联"
                  aria-label="清除弹幕关联"
                >
                  <Trash2 size={14} />
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {loadError && <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-600">{loadError}</p>}

      {!loadError && matched && (
        <div className="rounded-xl border border-gray-200 p-4">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-sand-500">
            {isLocalImport ? (
              <span>来源 本地导入文件{state?.local_format ? ` · ${state.local_format}` : ''}</span>
            ) : (
              <span>弹幕库 {state?.episode_id}</span>
            )}
            {state?.match_mode && <span>匹配方式 {matchModeLabel(state.match_mode)}</span>}
            <span className="flex items-center gap-1">
              时间轴偏移
              <button
                type="button"
                onClick={() => void adjustOffset(-0.5)}
                disabled={!isAdmin || busy === 'offset'}
                className="btn-ghost h-7 w-7 justify-center p-0"
                aria-label="弹幕提前 0.5 秒"
              >
                <Minus size={12} />
              </button>
              <span className="min-w-12 text-center font-medium text-ink-600">
                {formatOffset(Number(state?.shift || 0))}
              </span>
              <button
                type="button"
                onClick={() => void adjustOffset(0.5)}
                disabled={!isAdmin || busy === 'offset'}
                className="btn-ghost h-7 w-7 justify-center p-0"
                aria-label="弹幕延后 0.5 秒"
              >
                <Plus size={12} />
              </button>
              {Number(state?.shift || 0) !== 0 && (
                <button
                  type="button"
                  onClick={() => void resetOffset()}
                  disabled={!isAdmin || busy === 'offset'}
                  className="btn-ghost h-7 gap-1 px-2 text-xs"
                >
                  <RotateCcw size={12} />
                  归零
                </button>
              )}
            </span>
          </div>
          {isAdmin && (
            <div className="mt-2 flex flex-wrap gap-2">
              {OFFSET_STEPS.map((step) => (
                <button
                  key={step}
                  type="button"
                  onClick={() => void adjustOffset(step)}
                  disabled={busy === 'offset'}
                  className="btn-outline h-8 px-2 text-xs"
                >
                  {step > 0 ? `+${step}s` : `${step}s`}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {!loadError && !matched && !loading && (
        <div className="rounded-xl border border-amber-200 bg-amber-50/60 p-4 text-xs text-amber-800">
          <p>
            {status === 'failed'
              ? '弹幕服务本次回源失败，已记录状态。可稍后重试，或改用手动搜索。'
              : '这一集还没有匹配到弹幕库。'}
          </p>
          {state?.attempts && state.attempts.length > 0 && (
            <ul className="mt-2 space-y-1">
              {state.attempts.map((attempt, index) => (
                <li key={`${attempt.source}-${attempt.mode}-${index}`}>
                  {attempt.source} · {attemptModeLabel(attempt.mode)} · {attemptOutcomeLabel(attempt.outcome)}
                  {attempt.error ? `（${attempt.error}）` : ''}
                </li>
              ))}
            </ul>
          )}
          {!isAdmin && <p className="mt-2 text-amber-700">需要管理员权限才能手动匹配。</p>}
        </div>
      )}

      {prewarm ? (
        <div className="rounded-xl border border-gray-200 p-4 text-xs text-sand-500">
          <p>
            整季预热 {prewarm.status === 'completed' ? '已完成' : prewarm.status === 'failed' ? '失败' : '进行中'}：
            {prewarm.processed}/{prewarm.total} 集 · 有弹幕 {prewarm.matched} · 空 {prewarm.empty} · 命中缓存{' '}
            {prewarm.cached}
            {prewarm.failed ? ` · 失败 ${prewarm.failed}` : ''}
            {prewarm.current_episode ? ` · 当前 ${prewarm.current_episode}` : ''}
          </p>
          {prewarm.error ? <p className="mt-1 text-red-600">{prewarm.error}</p> : null}
          {prewarm.details.some((detail) => detail.error) ? (
            <ul className="mt-2 space-y-1 text-amber-700">
              {prewarm.details
                .filter((detail) => detail.error)
                .map((detail) => (
                  <li key={detail.media_id}>
                    {detail.episode_key || detail.media_id}：{detail.error}
                  </li>
                ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      {searchOpen &&
        createPortal(
          <DanmakuSearchDialog
            mediaId={selectedMediaId}
            busy={busy === 'apply'}
            onApply={applyCandidate}
            onClose={() => setSearchOpen(false)}
          />,
          document.body,
        )}
    </section>
  )
}

type DanmakuSearchDialogProps = {
  mediaId: string
  busy: boolean
  onApply: (episodeId: string, animeTitle: string, episodeTitle: string) => void
  onClose: () => void
}

function DanmakuSearchDialog({ mediaId, busy, onApply, onClose }: DanmakuSearchDialogProps) {
  const [keyword, setKeyword] = useState('')
  const [episode, setEpisode] = useState('')
  const [searching, setSearching] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<DanmakuSearchResult | null>(null)

  const runSearch = useCallback(async () => {
    const trimmed = keyword.trim()
    if (!trimmed) {
      setError('请输入番剧名称')
      return
    }
    setSearching(true)
    setError('')
    try {
      const parsedEpisode = Number.parseInt(episode, 10)
      setResult(await danmakuAPI.search(trimmed, Number.isFinite(parsedEpisode) && parsedEpisode > 0 ? parsedEpisode : undefined))
    } catch (searchError) {
      setResult(null)
      setError(describeDanmakuError(searchError).message)
    } finally {
      setSearching(false)
    }
  }, [episode, keyword])

  const animes = useMemo(
    () => (result?.results || []).flatMap((entry) => entry.animes.map((anime) => ({ source: entry.source, anime }))),
    [result],
  )

  return (
    <div
      className="fixed inset-0 z-[120] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label="搜索弹幕"
    >
      <div className="flex max-h-[min(86vh,820px)] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex shrink-0 items-center gap-3 border-b border-gray-200 px-5 py-4">
          <div className="min-w-0 flex-1">
            <h3 className="font-semibold text-ink-700">手动匹配弹幕</h3>
            <p className="mt-1 truncate text-xs text-sand-500">
              关键词会交给弹幕源搜索（默认带集数过滤），选中后写入这一集的关联：{mediaId}
            </p>
          </div>
          <button type="button" onClick={onClose} className="btn-ghost h-9 w-9 justify-center p-0" aria-label="关闭弹幕搜索">
            <X size={17} />
          </button>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-gray-200 px-5 py-3">
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') void runSearch()
            }}
            placeholder="番剧名称，例如：进击的巨人"
            className="h-9 min-w-60 flex-1 rounded-lg border border-gray-200 px-3 text-sm outline-none focus:border-brand-400"
            aria-label="弹幕搜索关键词"
          />
          <input
            value={episode}
            onChange={(event) => setEpisode(event.target.value.replace(/[^0-9]/g, ''))}
            placeholder="集数（可选）"
            className="h-9 w-28 rounded-lg border border-gray-200 px-3 text-sm outline-none focus:border-brand-400"
            aria-label="弹幕搜索集数"
          />
          <button type="button" onClick={() => void runSearch()} disabled={searching} className="btn-outline h-9 gap-1.5 px-3 text-xs">
            {searching ? <LoaderCircle size={14} className="animate-spin" /> : <Search size={14} />}
            搜索
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {error && <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-600">{error}</p>}
          {!error && !searching && result && animes.length === 0 && (
            <p className="py-20 text-center text-sm text-sand-500">
              没有搜索到结果。可换用原名，或只填关键词不带集数。
              {result.errors.length > 0 ? `（${result.errors.map((entry) => `${entry.source}: ${entry.error}`).join('；')}）` : ''}
            </p>
          )}
          {!searching && animes.length > 0 && (
            <div className="space-y-3">
              {animes.map(({ source, anime }) => (
                <div key={`${source}-${anime.anime_id}-${anime.anime_title}`} className="rounded-xl border border-gray-200 p-4">
                  <p className="truncate text-sm font-medium text-ink-700" title={anime.anime_title}>
                    {anime.anime_title}
                  </p>
                  <p className="mt-1 text-xs text-sand-500">
                    {[source, anime.type_description].filter(Boolean).join(' · ')}
                  </p>
                  {(anime.episodes || []).length > 0 && (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {(anime.episodes || []).map((entry) => (
                        <button
                          key={entry.episode_id}
                          type="button"
                          disabled={busy}
                          onClick={() => onApply(entry.episode_id, anime.anime_title, entry.episode_title || '')}
                          className="btn-outline h-8 px-2 text-xs"
                          title={`弹幕库 ${entry.episode_id}`}
                        >
                          {entry.episode_number ? `第 ${entry.episode_number} 集` : entry.episode_title || entry.episode_id}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function matchModeLabel(mode: string): string {
  return (
    { tmdb: 'TMDB', filename: '文件名', title: '标题', hash: '文件哈希', manual: '手动指定', import: '本地导入' }[mode] ||
    mode
  )
}

function attemptModeLabel(mode: string): string {
  return { tmdb: 'TMDB 反查', filename: '文件名匹配', title: '标题搜索' }[mode] || mode
}

function attemptOutcomeLabel(outcome: string): string {
  return { matched: '命中', no_candidates: '无候选', error: '出错' }[outcome] || outcome
}

function formatOffset(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds === 0) return '0s'
  return `${seconds > 0 ? '+' : ''}${Math.round(seconds * 10) / 10}s`
}
