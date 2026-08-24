import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import toast from 'react-hot-toast'
import { Ban, Captions, Eye, Info, Languages, LoaderCircle, Pencil, RefreshCw, Search, Sparkles, Trash2, X } from 'lucide-react'

import {
  subtitlesAPI,
  type SubtitleCandidatePreview,
  type SubtitleASRProfile,
  type SubtitleASRSourceLanguage,
  type SubtitleASRTask,
  type SubtitleSearchCandidate,
  type SubtitleSearchResponse,
  type SubtitleTrack,
} from '../api/subtitles'
import { confirmAction } from '../components/confirmAction'
import { useAuthStore } from '../stores/auth'
import type { MediaVersion } from '../types'
import { mediaFilename } from '../utils/mediaFilename'
import { subtitleASRModelLabel, subtitleASRProgressLabel, subtitleASRStageLabel } from './subtitleASRTaskModel'
import { SubHDCandidateDetails } from './SubHDCandidateDetails'

type MediaDetailSubtitlesProps = {
  mediaId: string
  versions: MediaVersion[]
  versionsLoading: boolean
}

type PreviewState = {
  title: string
  content: string
  loading: boolean
  error: string
} | null

export function MediaDetailSubtitles({ mediaId, versions, versionsLoading }: MediaDetailSubtitlesProps) {
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const defaultMediaId = useMemo(
    () => versions.find((version) => version.is_current)?.id || mediaId,
    [mediaId, versions],
  )
  const [selectedMediaId, setSelectedMediaId] = useState(defaultMediaId)
  const selectedMediaIdRef = useRef(selectedMediaId)
  const [tracks, setTracks] = useState<SubtitleTrack[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [deletingPath, setDeletingPath] = useState('')
  const [syncingCloud, setSyncingCloud] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState('')
  const [searchResult, setSearchResult] = useState<SubtitleSearchResponse | null>(null)
  const [preview, setPreview] = useState<PreviewState>(null)
  const [applyingCandidateId, setApplyingCandidateId] = useState('')
  const [asrLanguage, setAsrLanguage] = useState<SubtitleASRSourceLanguage>('ja')
  const [asrModels, setAsrModels] = useState<string[]>([])
  const [asrModel, setAsrModel] = useState('')
  const [asrModelsError, setAsrModelsError] = useState('')
  const [asrProfiles, setAsrProfiles] = useState<SubtitleASRProfile[]>([])
  const [asrProfileKey, setAsrProfileKey] = useState('')
  const [asrProfilesError, setAsrProfilesError] = useState('')
  const [asrCreating, setAsrCreating] = useState(false)
  const [asrUpdating, setAsrUpdating] = useState(false)
  const [asrTask, setAsrTask] = useState<SubtitleASRTask | null>(null)
  const [asrError, setAsrError] = useState('')
  const announcedASRTask = useRef('')
  const asrPollingTaskID = asrTask?.id
  const asrPolling = asrTask?.status === 'queued' || asrTask?.status === 'running'
  const selectedASRProfile = useMemo(
    () => asrProfiles.find((profile) => asrProfileValue(profile) === asrProfileKey) ?? null,
    [asrProfileKey, asrProfiles],
  )
  const selectedVersion = useMemo(
    () => versions.find((version) => version.id === selectedMediaId),
    [selectedMediaId, versions],
  )
  const canSyncCloudSubtitles = isAdmin && Boolean(selectedVersion?.path?.toLowerCase().startsWith('cloud://'))
  const asrCapabilityHint = Array.from(new Set([asrProfilesError, asrModelsError].filter(Boolean))).join(' · ')

  useEffect(() => {
    selectedMediaIdRef.current = selectedMediaId
  }, [selectedMediaId])

  useEffect(() => {
    subtitlesAPI.listASRProfiles()
      .then((profiles) => {
        setAsrProfiles(profiles)
        setAsrProfileKey((current) => current || (profiles[0] ? asrProfileValue(profiles[0]) : ''))
        setAsrProfilesError(profiles.length > 0 ? '' : '没有可用的 AI 字幕翻译模型')
      })
      .catch((error) => setAsrProfilesError(errorMessage(error, 'AI 字幕翻译模型加载失败')))
  }, [])

  useEffect(() => {
    subtitlesAPI.listASRModels()
      .then((models) => {
        setAsrModels(models)
        setAsrModel((current) => current || models[0] || '')
        setAsrModelsError(models.length > 0 ? '' : '没有可用的 ASR 识别模型')
      })
      .catch((error) => setAsrModelsError(errorMessage(error, 'ASR 识别模型加载失败')))
  }, [])

  useEffect(() => {
    if (!versions.some((version) => version.id === selectedMediaId)) {
      setSelectedMediaId(defaultMediaId)
    }
  }, [defaultMediaId, selectedMediaId, versions])

  const loadTracks = useCallback(async () => {
    if (!selectedMediaId) return
    setLoading(true)
    setLoadError('')
    try {
      setTracks(await subtitlesAPI.list(selectedMediaId))
    } catch (error) {
      setTracks([])
      setLoadError(errorMessage(error, '字幕文件加载失败'))
    } finally {
      setLoading(false)
    }
  }, [selectedMediaId])

  useEffect(() => {
    void loadTracks()
    setSearchOpen(false)
    setSearchResult(null)
    setPreview(null)
    setAsrTask(null)
    setAsrError('')
  }, [loadTracks])

  useEffect(() => {
    if (!asrPollingTaskID || !asrPolling || asrUpdating) return
    let stopped = false
    let timer = 0
    const poll = async () => {
      try {
        const task = await subtitlesAPI.getASR(selectedMediaId, asrPollingTaskID)
        if (stopped) return
        setAsrTask(task)
        setAsrError('')
        if (['queued', 'running'].includes(task.status)) {
          timer = window.setTimeout(() => void poll(), 1500)
        }
      } catch (error) {
        if (stopped) return
        setAsrError(errorMessage(error, 'AI 字幕任务状态读取失败'))
        timer = window.setTimeout(() => void poll(), 3000)
      }
    }
    timer = window.setTimeout(() => void poll(), 1000)
    return () => {
      stopped = true
      window.clearTimeout(timer)
    }
  }, [asrPolling, asrPollingTaskID, asrUpdating, selectedMediaId])

  useEffect(() => {
    if (!asrTask || announcedASRTask.current === asrTask.id) return
    if (asrTask.status === 'completed') {
      announcedASRTask.current = asrTask.id
      toast.success('AI 简体中文字幕已保存到当前片源')
      void loadTracks()
    } else if (asrTask.status === 'failed') {
      announcedASRTask.current = asrTask.id
      toast.error(asrTask.error || 'AI 字幕生成失败')
    }
  }, [asrTask, loadTracks])

  const openExistingPreview = useCallback(async (track: SubtitleTrack) => {
    setPreview({ title: track.name || track.label, content: '', loading: true, error: '' })
    try {
      const content = await subtitlesAPI.previewExisting(selectedMediaId, track.path)
      setPreview({ title: track.name || track.label, content, loading: false, error: '' })
    } catch (error) {
      setPreview({ title: track.name || track.label, content: '', loading: false, error: errorMessage(error, '字幕预览失败') })
    }
  }, [selectedMediaId])

  const deleteTrack = useCallback(async (track: SubtitleTrack) => {
    if (deletingPath) return
    const confirmed = await confirmAction({
      title: '删除字幕文件',
      message: `确定删除“${track.name || track.label}”吗？此操作会删除实际字幕文件，无法撤销。`,
      confirmText: '删除字幕',
    })
    if (!confirmed) return
    setDeletingPath(track.path)
    try {
      await subtitlesAPI.delete(selectedMediaId, track.path)
      toast.success('字幕文件已删除')
      await loadTracks()
    } catch (error) {
      toast.error(errorMessage(error, '字幕删除失败'))
    } finally {
      setDeletingPath('')
    }
  }, [deletingPath, loadTracks, selectedMediaId])

  const syncCloudSubtitles = useCallback(async () => {
    if (syncingCloud || !canSyncCloudSubtitles) return
    const targetMediaId = selectedMediaId
    setSyncingCloud(true)
    try {
      const result = await subtitlesAPI.refreshCloud(targetMediaId)
      toast.success(result.cached > 0 ? `已同步 ${result.cached} 条云盘字幕` : '云盘中未发现匹配字幕')
      if (selectedMediaIdRef.current === targetMediaId) {
        await loadTracks()
      }
    } catch (error) {
      toast.error(errorMessage(error, '云盘字幕同步失败'))
    } finally {
      setSyncingCloud(false)
    }
  }, [canSyncCloudSubtitles, loadTracks, selectedMediaId, syncingCloud])

  const searchCandidates = useCallback(async () => {
    setSearching(true)
    setSearchError('')
    setSearchResult(null)
    try {
      setSearchResult(await subtitlesAPI.search(selectedMediaId))
    } catch (error) {
      setSearchError(errorMessage(error, '字幕搜索失败'))
    } finally {
      setSearching(false)
    }
  }, [selectedMediaId])

  const openSearch = useCallback(() => {
    setSearchOpen(true)
    void searchCandidates()
  }, [searchCandidates])

  const previewCandidate = useCallback(async (candidate: SubtitleSearchCandidate) => {
    if (!searchResult) return
    setPreview({ title: candidate.filename || candidate.title, content: '', loading: true, error: '' })
    try {
      const result: SubtitleCandidatePreview = await subtitlesAPI.previewCandidate(
        selectedMediaId,
        searchResult.session_id,
        candidate.candidate_id,
      )
      setPreview({
        title: result.filename || result.title,
        content: result.content_sample,
        loading: false,
        error: result.content_sample ? '' : '字幕源返回了空预览内容',
      })
    } catch (error) {
      setPreview({ title: candidate.filename || candidate.title, content: '', loading: false, error: errorMessage(error, '临时预览失败') })
    }
  }, [searchResult, selectedMediaId])

  const applyCandidate = useCallback(async (candidate: SubtitleSearchCandidate) => {
    if (!searchResult || applyingCandidateId) return
    setApplyingCandidateId(candidate.candidate_id)
    try {
      await subtitlesAPI.applyCandidate(selectedMediaId, searchResult.session_id, candidate.candidate_id)
      toast.success('字幕已保存到当前片源')
      await loadTracks()
      setSearchOpen(false)
    } catch (error) {
      toast.error(errorMessage(error, '字幕保存失败'))
    } finally {
      setApplyingCandidateId('')
    }
  }, [applyingCandidateId, loadTracks, searchResult, selectedMediaId])

  const createASR = useCallback(async () => {
    if (asrCreating || !asrModel || !selectedASRProfile || ['queued', 'running'].includes(asrTask?.status || '')) return
    setAsrCreating(true)
    setAsrError('')
    try {
      const task = await subtitlesAPI.createASR(selectedMediaId, asrLanguage, asrModel, selectedASRProfile)
      announcedASRTask.current = ''
      setAsrTask(task)
    } catch (error) {
      const message = errorMessage(error, 'AI 字幕任务创建失败')
      setAsrError(message)
      toast.error(message)
    } finally {
      setAsrCreating(false)
    }
  }, [asrCreating, asrLanguage, asrModel, asrTask?.status, selectedASRProfile, selectedMediaId])

  const updateQueuedASRModel = useCallback(async () => {
    if (asrUpdating || asrTask?.status !== 'queued' || !asrModel || !selectedASRProfile) return
    setAsrUpdating(true)
    setAsrError('')
    try {
      setAsrTask(await subtitlesAPI.updateASRModel(asrTask.id, asrModel, selectedASRProfile))
      toast.success('排队任务已改用所选模型')
    } catch (error) {
      const message = errorMessage(error, '字幕任务模型修改失败')
      setAsrError(message)
      toast.error(message)
    } finally {
      setAsrUpdating(false)
    }
  }, [asrModel, asrTask, asrUpdating, selectedASRProfile])

  const cancelQueuedASR = useCallback(async () => {
    if (asrUpdating || asrTask?.status !== 'queued') return
    if (!(await confirmAction({
      title: '撤销排队任务',
      message: '确定撤销该字幕任务吗？已生成的任务缓存会一并清理。',
      confirmText: '撤销任务',
    }))) return
    setAsrUpdating(true)
    setAsrError('')
    try {
      setAsrTask(await subtitlesAPI.cancelASR(asrTask.id))
      toast.success('排队任务已撤销')
    } catch (error) {
      const message = errorMessage(error, '字幕任务撤销失败')
      setAsrError(message)
      toast.error(message)
    } finally {
      setAsrUpdating(false)
    }
  }, [asrTask, asrUpdating])

  const retranslateASR = useCallback(async () => {
    if (asrUpdating || asrTask?.status !== 'completed' || !asrTask.cached_audio || !asrTask.cached_transcript || !selectedASRProfile) return
    setAsrUpdating(true)
    setAsrError('')
    try {
      announcedASRTask.current = ''
      setAsrTask(await subtitlesAPI.retranslateASR(asrTask.id, selectedASRProfile))
      toast.success('已复用音轨和 ASR 结果重新翻译')
    } catch (error) {
      const message = errorMessage(error, '字幕重新翻译失败')
      setAsrError(message)
      toast.error(message)
    } finally {
      setAsrUpdating(false)
    }
  }, [asrTask, asrUpdating, selectedASRProfile])

  return (
    <section className="space-y-3" aria-label="字幕文件">
      <div className="flex flex-wrap items-center gap-2">
        <Captions size={16} className="text-[#c9954a]" />
        <h2 className="text-sm font-semibold text-ink-600">字幕文件</h2>
        <span className="text-xs text-sand-500">{loading ? '检查中' : tracks.length > 0 ? `${tracks.length} 个` : '无字幕'}</span>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {versions.length > 1 && (
            <select
              value={selectedMediaId}
              disabled={versionsLoading}
              onChange={(event) => setSelectedMediaId(event.target.value)}
              className="h-9 max-w-60 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
              aria-label="选择字幕所属片源版本"
            >
              {versions.map((version) => (
                <option key={version.id} value={version.id}>{versionOptionLabel(version)}</option>
              ))}
            </select>
          )}
          {canSyncCloudSubtitles && (
            <button
              type="button"
              onClick={() => void syncCloudSubtitles()}
              disabled={syncingCloud}
              className="btn-outline h-9 gap-1.5 px-3 text-xs"
            >
              <RefreshCw size={14} className={syncingCloud ? 'animate-spin' : ''} />
              同步云盘字幕
            </button>
          )}
          <button
            type="button"
            onClick={() => void loadTracks()}
            disabled={loading}
            className="btn-outline h-9 w-9 justify-center p-0"
            title="重新检查字幕文件"
            aria-label="重新检查字幕文件"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          <button type="button" onClick={openSearch} className="btn-outline h-9 gap-1.5 px-3 text-xs">
            <Search size={14} />
            搜索字幕
          </button>
          <select
            value={asrModel}
            disabled={asrCreating || asrUpdating || asrModels.length === 0 || asrTask?.status === 'running'}
            onChange={(event) => setAsrModel(event.target.value)}
            className="h-9 max-w-60 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
            aria-label="ASR 识别模型"
          >
            {asrModels.map((model) => (
              <option key={model} value={model}>{subtitleASRModelLabel(model)}</option>
            ))}
          </select>
          <select
            value={asrProfileKey}
            disabled={asrCreating || asrUpdating || asrProfiles.length === 0 || asrTask?.status === 'running'}
            onChange={(event) => setAsrProfileKey(event.target.value)}
            className="h-9 max-w-72 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
            aria-label="AI 字幕翻译模型"
          >
            {asrProfiles.map((profile) => (
              <option key={asrProfileValue(profile)} value={asrProfileValue(profile)}>
                {profile.provider_label} · {profile.model}
              </option>
            ))}
          </select>
          <select
            value={asrLanguage}
            disabled={asrCreating || asrUpdating || !asrModel || !selectedASRProfile || ['queued', 'running'].includes(asrTask?.status || '')}
            onChange={(event) => setAsrLanguage(event.target.value as SubtitleASRSourceLanguage)}
            className="h-9 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
            aria-label="AI 字幕源语言"
          >
            <option value="auto">自动识别</option>
            <option value="ja">日语</option>
            <option value="en">英语</option>
            <option value="zh">中文</option>
            <option value="ko">韩语</option>
          </select>
          <button
            type="button"
            onClick={() => void createASR()}
            disabled={asrCreating || asrUpdating || !asrModel || !selectedASRProfile || ['queued', 'running'].includes(asrTask?.status || '')}
            className="btn-outline h-9 gap-1.5 px-3 text-xs"
          >
            {asrCreating || ['queued', 'running'].includes(asrTask?.status || '')
              ? <LoaderCircle size={14} className="animate-spin" />
              : <Sparkles size={14} />}
            AI 生成字幕
          </button>
        </div>
      </div>

      {(asrTask || asrError) && (
        <div className={`rounded-lg px-3 py-2 text-xs ${asrTask?.status === 'failed' || asrError ? 'bg-red-50 text-red-600' : 'bg-brand-50 text-ink-600'}`}>
          {asrTask && asrTask.status !== 'failed' && (
            <span>
              {subtitleASRStageLabel(asrTask.stage)}
              {subtitleASRProgressLabel(asrTask) ? ` · ${subtitleASRProgressLabel(asrTask)}` : ''}
              {asrTask.status === 'completed' && asrTask.result ? ` · ${asrTask.result.segment_count} 个分段` : ''}
            </span>
          )}
          {asrTask?.status === 'failed' && <span>{asrTask.error || 'AI 字幕生成失败'}</span>}
          {asrError && <span>{asrTask ? ' · ' : ''}{asrError}</span>}
          {asrTask?.status === 'queued' && (
            <span className="ml-2 inline-flex gap-1 align-middle">
              <button
                type="button"
                onClick={() => void updateQueuedASRModel()}
                disabled={asrUpdating || !asrModel || !selectedASRProfile}
                className="btn-outline h-8 w-8 justify-center p-0"
                title="改用当前选择的模型"
                aria-label="修改排队任务模型"
              >
                {asrUpdating ? <LoaderCircle size={13} className="animate-spin" /> : <Pencil size={13} />}
              </button>
              <button
                type="button"
                onClick={() => void cancelQueuedASR()}
                disabled={asrUpdating}
                className="btn-outline h-8 w-8 justify-center p-0 !border-red-100 !text-red-500"
                title="撤销排队任务"
                aria-label="撤销排队字幕任务"
              >
                {asrUpdating ? <LoaderCircle size={13} className="animate-spin" /> : <Ban size={13} />}
              </button>
            </span>
          )}
          {asrTask?.status === 'completed' && asrTask.cached_audio && asrTask.cached_transcript && (
            <button
              type="button"
              onClick={() => void retranslateASR()}
              disabled={asrUpdating || !selectedASRProfile}
              className="btn-outline ml-2 inline-flex h-8 gap-1.5 px-2 text-xs align-middle"
              title="复用音轨和 ASR 结果"
            >
              {asrUpdating ? <LoaderCircle size={13} className="animate-spin" /> : <Languages size={13} />}
              用所选模型重新翻译
            </button>
          )}
        </div>
      )}
      {asrCapabilityHint && (
        <p className="mt-2 flex items-center gap-1.5 text-xs text-sand-500" title={asrCapabilityHint}>
          <Info size={13} className="shrink-0" />
          AI 字幕服务当前未就绪，需要使用时再启动本机 ASR。
        </p>
      )}

      {loadError && <p className="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600">{loadError}</p>}
      {!loading && !loadError && tracks.length === 0 && (
        <p className="rounded-xl border border-dashed border-gray-200 bg-gray-50/60 px-4 py-4 text-sm text-sand-500">
          当前片源未发现外挂字幕文件。
        </p>
      )}
      {tracks.length > 0 && (
        <div className="divide-y divide-gray-200 border-y border-gray-200">
          {tracks.map((track) => (
            <div key={track.path} className="flex items-center gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-ink-600" title={track.name}>{track.name || track.label}</p>
                <p className="mt-1 text-xs text-sand-500">
                  {[track.label, track.codec?.toUpperCase(), subtitleSourceLabel(track.source)].filter(Boolean).join(' · ')}
                </p>
              </div>
              <button
                type="button"
                onClick={() => void openExistingPreview(track)}
                className="btn-outline h-9 w-9 justify-center p-0"
                title="预览字幕"
                aria-label={`预览字幕 ${track.name || track.label}`}
              >
                <Eye size={14} />
              </button>
              {track.source?.toLowerCase() !== 'cloud-media' && (
                <button
                  type="button"
                  onClick={() => void deleteTrack(track)}
                  disabled={Boolean(deletingPath)}
                  className="btn-outline h-9 w-9 justify-center p-0 !border-red-100 !text-red-500 hover:!border-red-200 hover:!bg-red-50"
                  title="删除字幕"
                  aria-label={`删除字幕 ${track.name || track.label}`}
                >
                  {deletingPath === track.path ? <LoaderCircle size={14} className="animate-spin" /> : <Trash2 size={14} />}
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {searchOpen && createPortal(
        <SubtitleSearchDialog
          searching={searching}
          error={searchError}
          result={searchResult}
          applyingCandidateId={applyingCandidateId}
          onRefresh={() => void searchCandidates()}
          onPreview={(candidate) => void previewCandidate(candidate)}
          onApply={(candidate) => void applyCandidate(candidate)}
          onClose={() => setSearchOpen(false)}
        />,
        document.body,
      )}
      {preview && createPortal(
        <SubtitlePreviewDialog preview={preview} onClose={() => setPreview(null)} />,
        document.body,
      )}
    </section>
  )
}

function SubtitleSearchDialog({
  searching,
  error,
  result,
  applyingCandidateId,
  onRefresh,
  onPreview,
  onApply,
  onClose,
}: {
  searching: boolean
  error: string
  result: SubtitleSearchResponse | null
  applyingCandidateId: string
  onRefresh: () => void
  onPreview: (candidate: SubtitleSearchCandidate) => void
  onApply: (candidate: SubtitleSearchCandidate) => void
  onClose: () => void
}) {
  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label="搜索字幕">
      <div className="flex max-h-[min(86vh,820px)] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        <div className="flex shrink-0 items-center gap-3 border-b border-gray-200 px-5 py-4">
          <div className="min-w-0 flex-1">
            <h3 className="font-semibold text-ink-700">搜索匹配字幕</h3>
            <p className="mt-1 truncate text-xs text-sand-500">{result?.query ? `匹配词：${result.query}` : '由 pipeline 根据当前片源标题或番号匹配'}</p>
          </div>
          <button type="button" onClick={onRefresh} disabled={searching} className="btn-outline h-9 gap-1.5 px-3 text-xs">
            <RefreshCw size={13} className={searching ? 'animate-spin' : ''} />
            重新搜索
          </button>
          <button type="button" onClick={onClose} className="btn-ghost h-9 w-9 justify-center p-0" aria-label="关闭字幕搜索">
            <X size={17} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {searching && (
            <div className="flex items-center justify-center gap-2 py-20 text-sm text-sand-500">
              <LoaderCircle size={17} className="animate-spin" />
              正在搜索字幕
            </div>
          )}
          {!searching && error && <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-600">{error}</p>}
          {!searching && !error && result && result.items.length === 0 && (
            <p className="py-20 text-center text-sm text-sand-500">没有找到匹配的字幕文件。</p>
          )}
          {!searching && !error && result && result.items.length > 0 && (
            <div className="space-y-3">
              {result.items.map((candidate) => (
                <div key={candidate.candidate_id} className="rounded-xl border border-gray-200 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
                    <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-ink-700" title={candidate.filename || candidate.title}>
                      {candidate.filename || candidate.title || `字幕候选 ${candidate.rank}`}
                    </p>
                    <p className="mt-1 text-xs text-sand-500">
                      {[subtitleSourceLabel(candidate.provider), candidate.language, candidate.source_score > 0 ? `匹配分 ${candidate.source_score}` : ''].filter(Boolean).join(' · ')}
                    </p>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <button type="button" onClick={() => onPreview(candidate)} className="btn-outline h-9 gap-1.5 px-3 text-xs">
                        <Eye size={13} />
                        临时预览
                      </button>
                      <button
                        type="button"
                        onClick={() => onApply(candidate)}
                        disabled={Boolean(applyingCandidateId)}
                        className="btn-primary h-9 gap-1.5 px-3 text-xs"
                      >
                        {applyingCandidateId === candidate.candidate_id && <LoaderCircle size={13} className="animate-spin" />}
                        保存到作品
                      </button>
                    </div>
                  </div>
                  {candidate.provider === 'subhd' && <SubHDCandidateDetails candidate={candidate} />}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
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
          <button type="button" onClick={onClose} className="btn-ghost h-9 w-9 justify-center p-0" aria-label="关闭字幕预览">
            <X size={17} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto bg-gray-950 p-5">
          {preview.loading && (
            <div className="flex items-center justify-center gap-2 py-20 text-sm text-gray-300">
              <LoaderCircle size={17} className="animate-spin" />
              正在加载预览
            </div>
          )}
          {!preview.loading && preview.error && <p className="rounded-lg bg-red-950/60 px-4 py-3 text-sm text-red-200">{preview.error}</p>}
          {!preview.loading && preview.content && (
            <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-7 text-gray-100">{preview.content}</pre>
          )}
        </div>
      </div>
    </div>
  )
}

function versionOptionLabel(version: MediaVersion): string {
  const fileName = mediaFilename(version)
  return `${version.is_current ? '当前 · ' : ''}${fileName}`
}

function subtitleSourceLabel(source: string): string {
  return {
    media: '媒体目录',
    cache: '本地字幕缓存',
    subtitlecat: 'SubtitleCat',
    assrt: 'Assrt',
    opensubtitles: 'OpenSubtitles',
    openlist: 'OpenList',
    'cloud-media': '云盘本地字幕',
    '115': '115 网盘',
    'sensevoice-deepseek': 'SenseVoice + DeepSeek',
  }[source?.toLowerCase()] || source || '未知来源'
}

function asrProfileValue(profile: SubtitleASRProfile): string {
  return `${profile.provider}\n${profile.model}`
}

function errorMessage(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error
    || (error instanceof Error ? error.message : fallback)
}
