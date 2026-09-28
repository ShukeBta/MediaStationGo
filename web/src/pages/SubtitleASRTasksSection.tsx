import { useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'
import { Ban, Captions, Languages, LoaderCircle, Pencil, RotateCcw, Trash2 } from 'lucide-react'
import { Link } from 'react-router-dom'

import { subtitlesAPI, type SubtitleASRProfile, type SubtitleASRTask } from '../api/subtitles'
import { confirmAction } from '../components/confirmAction'
import {
  splitSubtitleASRTasks,
  subtitleASRLanguageLabel,
  subtitleASRModelLabel,
  subtitleASRProfileLabel,
  subtitleASRProgressLabel,
  subtitleASRResultSummary,
  subtitleASRStageLabel,
} from './subtitleASRTaskModel'

type SubtitleASRTasksSectionProps = {
  tasks: SubtitleASRTask[] | null
  error: string
  onChanged: () => Promise<void>
}

export function SubtitleASRTasksSection({ tasks, error, onChanged }: SubtitleASRTasksSectionProps) {
  const grouped = splitSubtitleASRTasks(tasks ?? [])
  const [profiles, setProfiles] = useState<SubtitleASRProfile[]>([])
  const [profileKey, setProfileKey] = useState('')
  const [profileError, setProfileError] = useState('')
  const [asrModels, setAsrModels] = useState<string[]>([])
  const [asrModel, setAsrModel] = useState('')
  const [asrModelError, setAsrModelError] = useState('')
  const [busyTaskID, setBusyTaskID] = useState('')
  const selectedProfile = useMemo(
    () => profiles.find((profile) => asrProfileKey(profile) === profileKey) ?? null,
    [profileKey, profiles],
  )

  useEffect(() => {
    subtitlesAPI.listASRProfiles()
      .then((items) => {
        setProfiles(items)
        setProfileKey((current) => current || (items[0] ? asrProfileKey(items[0]) : ''))
        setProfileError(items.length > 0 ? '' : '没有可用的本机或云端翻译模型')
      })
      .catch((cause) => setProfileError(errorMessage(cause, '翻译模型加载失败')))
  }, [])

  useEffect(() => {
    subtitlesAPI.listASRModels()
      .then((items) => {
        setAsrModels(items)
        setAsrModel((current) => current || items[0] || '')
        setAsrModelError(items.length > 0 ? '' : '没有可用的 ASR 识别模型')
      })
      .catch((cause) => setAsrModelError(errorMessage(cause, 'ASR 识别模型加载失败')))
  }, [])

  const retryTask = async (task: SubtitleASRTask) => {
    if (!asrModel || !selectedProfile || busyTaskID) return
    setBusyTaskID(task.id)
    try {
      await subtitlesAPI.retryASR(task.id, asrModel, selectedProfile)
      toast.success('字幕任务已按所选模型重新排队')
      await onChanged()
    } catch (cause) {
      toast.error(errorMessage(cause, '字幕任务重试失败'))
    } finally {
      setBusyTaskID('')
    }
  }

  const updateQueuedTaskModel = async (task: SubtitleASRTask) => {
    if (!asrModel || !selectedProfile || busyTaskID) return
    setBusyTaskID(task.id)
    try {
      await subtitlesAPI.updateASRModel(task.id, asrModel, selectedProfile)
      toast.success('排队任务已改用所选模型')
      await onChanged()
    } catch (cause) {
      toast.error(errorMessage(cause, '字幕任务模型修改失败'))
    } finally {
      setBusyTaskID('')
    }
  }

  const cancelTask = async (task: SubtitleASRTask) => {
    if (busyTaskID) return
    if (!(await confirmAction({
      title: '撤销排队任务',
      message: '确定撤销该字幕任务吗？已生成的任务缓存会一并清理。',
      confirmText: '撤销任务',
    }))) return
    setBusyTaskID(task.id)
    try {
      await subtitlesAPI.cancelASR(task.id)
      toast.success('排队任务已撤销')
      await onChanged()
    } catch (cause) {
      toast.error(errorMessage(cause, '字幕任务撤销失败'))
    } finally {
      setBusyTaskID('')
    }
  }

  const retranslateTask = async (task: SubtitleASRTask) => {
    if (!selectedProfile || busyTaskID) return
    setBusyTaskID(task.id)
    try {
      await subtitlesAPI.retranslateASR(task.id, selectedProfile)
      toast.success('已复用音轨和 ASR 结果重新翻译')
      await onChanged()
    } catch (cause) {
      toast.error(errorMessage(cause, '字幕重新翻译失败'))
    } finally {
      setBusyTaskID('')
    }
  }

  const deleteTask = async (task: SubtitleASRTask) => {
    if (busyTaskID) return
    if (!(await confirmAction({
      title: '删除字幕任务',
      message: '确定删除该字幕任务及其识别、翻译缓存吗？同一片源的共享音轨会保留，供后续任务复用。',
      confirmText: '删除任务',
    }))) return
    setBusyTaskID(task.id)
    try {
      await subtitlesAPI.deleteASR(task.id)
      toast.success('字幕任务及缓存已删除')
      await onChanged()
    } catch (cause) {
      toast.error(errorMessage(cause, '字幕任务删除失败'))
    } finally {
      setBusyTaskID('')
    }
  }

  return (
    <section className="glass-panel">
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Captions size={18} className="text-brand-500" />
        <h2 className="font-display text-lg font-semibold text-ink-600">AI 字幕生产任务</h2>
        <label className="ml-auto flex min-w-0 items-center gap-2 text-xs text-sand-500">
          识别模型
          <select
            value={asrModel}
            disabled={asrModels.length === 0}
            onChange={(event) => setAsrModel(event.target.value)}
            className="h-9 max-w-60 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
          >
            {asrModels.map((model) => (
              <option key={model} value={model}>{subtitleASRModelLabel(model)}</option>
            ))}
          </select>
        </label>
        <label className="flex min-w-0 items-center gap-2 text-xs text-sand-500">
          翻译模型
          <select
            value={profileKey}
            disabled={profiles.length === 0}
            onChange={(event) => setProfileKey(event.target.value)}
            className="h-9 max-w-80 rounded-lg border border-gray-200 bg-white px-3 text-xs text-ink-600 outline-none focus:border-brand-400"
          >
            {profiles.map((profile) => (
              <option key={asrProfileKey(profile)} value={asrProfileKey(profile)}>
                {profile.provider_label} · {profile.model}
              </option>
            ))}
          </select>
        </label>
      </div>
      {asrModelError && <p className="mb-3 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-700">{asrModelError}</p>}
      {profileError && <p className="mb-3 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-700">{profileError}</p>}
      {error && <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600">{error}</p>}
      {!error && tasks === null && <p className="text-sand-500">正在加载字幕任务…</p>}
      {!error && tasks !== null && (
        <div className="space-y-5">
          <div>
            <h3 className="mb-2 text-sm font-semibold text-ink-500">运行中</h3>
            <SubtitleASRTaskTable
              tasks={grouped.active}
              empty="暂无正在生产的字幕。"
              busyTaskID={busyTaskID}
              modelActionsEnabled={Boolean(asrModel && selectedProfile)}
              onUpdateModel={(task) => void updateQueuedTaskModel(task)}
              onCancel={(task) => void cancelTask(task)}
            />
          </div>
          <div>
            <h3 className="mb-2 text-sm font-semibold text-ink-500">最近完成</h3>
            <SubtitleASRTaskTable
              tasks={grouped.recent}
              empty="暂无已结束的字幕任务。"
              busyTaskID={busyTaskID}
              retryEnabled={Boolean(asrModel && selectedProfile)}
              retranslateEnabled={Boolean(selectedProfile)}
              onRetry={(task) => void retryTask(task)}
              onRetranslate={(task) => void retranslateTask(task)}
              onDelete={(task) => void deleteTask(task)}
            />
          </div>
        </div>
      )}
    </section>
  )
}

function SubtitleASRTaskTable({
  tasks,
  empty,
  busyTaskID,
  retryEnabled = false,
  retranslateEnabled = false,
  modelActionsEnabled = false,
  onRetry,
  onUpdateModel,
  onCancel,
  onRetranslate,
  onDelete,
}: {
  tasks: SubtitleASRTask[]
  empty: string
  busyTaskID: string
  retryEnabled?: boolean
  retranslateEnabled?: boolean
  modelActionsEnabled?: boolean
  onRetry?: (task: SubtitleASRTask) => void
  onUpdateModel?: (task: SubtitleASRTask) => void
  onCancel?: (task: SubtitleASRTask) => void
  onRetranslate?: (task: SubtitleASRTask) => void
  onDelete?: (task: SubtitleASRTask) => void
}) {
  if (tasks.length === 0) return <p className="text-sand-500">{empty}</p>
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[1080px] text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="py-2">作品 / 片源</th>
            <th>源语言</th>
            <th>阶段</th>
            <th>模型 / 缓存</th>
            <th>状态</th>
            <th>结果</th>
            <th>时间</th>
            {(onRetry || onUpdateModel || onCancel || onRetranslate || onDelete) && <th className="text-right">操作</th>}
          </tr>
        </thead>
        <tbody>
          {tasks.map((task) => (
            <tr key={task.id} className="border-t border-gray-200 align-top">
              <td className="max-w-sm py-2">
                {task.media_available ? (
                  <Link className="font-medium text-brand-600 hover:text-brand-500" to={`/media/${task.media_id}`}>
                    {task.media_title || task.media_filename || task.media_id}
                  </Link>
                ) : (
                  <span className="font-medium text-red-500">媒体记录已不存在</span>
                )}
                <div className="truncate text-xs text-sand-500" title={task.media_filename || task.media_id}>
                  {task.media_filename || task.media_id}
                </div>
              </td>
              <td className="text-ink-100">{subtitleASRLanguageLabel(task.source_language)}</td>
              <td className="text-ink-100">
                <div>{subtitleASRStageLabel(task.stage)}</div>
                {subtitleASRProgressLabel(task) && (
                  <div className="mt-1 text-xs text-sand-500">{subtitleASRProgressLabel(task)}</div>
                )}
              </td>
              <td className="max-w-72 text-xs text-ink-100">
                <div className="break-words">{subtitleASRProfileLabel(task)}</div>
                {(task.cached_audio || task.cached_transcript) && (
                  <div className="mt-1 text-emerald-600">
                    {[task.cached_audio ? '音轨已缓存' : '', task.cached_transcript ? '识别结果已缓存' : ''].filter(Boolean).join(' · ')}
                  </div>
                )}
              </td>
              <td>{subtitleASRStatusBadge(task.status)}</td>
              <td className={task.status === 'failed' ? 'max-w-md break-words text-red-600' : 'max-w-md break-words text-ink-100'}>
                {subtitleASRResultSummary(task)}
              </td>
              <td className="whitespace-nowrap text-ink-100">{subtitleASRTaskTime(task)}</td>
              {(onRetry || onUpdateModel || onCancel || onRetranslate || onDelete) && (
                <td>
                  <div className="flex justify-end gap-1">
                    {task.status === 'queued' && onUpdateModel && (
                      <button
                        type="button"
                        onClick={() => onUpdateModel(task)}
                        disabled={!modelActionsEnabled || Boolean(busyTaskID)}
                        className="btn-outline h-9 w-9 justify-center p-0"
                        title="改用所选模型"
                        aria-label="修改排队任务模型"
                      >
                        {busyTaskID === task.id ? <LoaderCircle size={14} className="animate-spin" /> : <Pencil size={14} />}
                      </button>
                    )}
                    {task.status === 'queued' && onCancel && (
                      <button
                        type="button"
                        onClick={() => onCancel(task)}
                        disabled={Boolean(busyTaskID)}
                        className="btn-outline h-9 w-9 justify-center p-0 !border-red-100 !text-red-500"
                        title="撤销排队任务"
                        aria-label="撤销排队字幕任务"
                      >
                        {busyTaskID === task.id ? <LoaderCircle size={14} className="animate-spin" /> : <Ban size={14} />}
                      </button>
                    )}
                    {task.status === 'failed' && onRetry && (
                      <button
                        type="button"
                        onClick={() => onRetry(task)}
                        disabled={!retryEnabled || Boolean(busyTaskID)}
                        className="btn-outline h-9 w-9 justify-center p-0"
                        title="使用所选模型重试"
                        aria-label="重试字幕任务"
                      >
                        {busyTaskID === task.id ? <LoaderCircle size={14} className="animate-spin" /> : <RotateCcw size={14} />}
                      </button>
                    )}
                    {task.status === 'completed' && task.cached_audio && task.cached_transcript && onRetranslate && (
                      <button
                        type="button"
                        onClick={() => onRetranslate(task)}
                        disabled={!retranslateEnabled || Boolean(busyTaskID)}
                        className="btn-outline h-9 w-9 justify-center p-0"
                        title="复用缓存并按所选模型重新翻译"
                        aria-label="使用其他模型重新翻译字幕"
                      >
                        {busyTaskID === task.id ? <LoaderCircle size={14} className="animate-spin" /> : <Languages size={14} />}
                      </button>
                    )}
                    {onDelete && (
                      <button
                        type="button"
                        onClick={() => onDelete(task)}
                        disabled={Boolean(busyTaskID)}
                        className="btn-outline h-9 w-9 justify-center p-0 !border-red-100 !text-red-500"
                        title="删除任务及任务级缓存"
                        aria-label="删除字幕任务"
                      >
                        {busyTaskID === task.id ? <LoaderCircle size={14} className="animate-spin" /> : <Trash2 size={14} />}
                      </button>
                    )}
                  </div>
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function subtitleASRStatusBadge(status: SubtitleASRTask['status']) {
  const styles = {
    queued: 'border-gray-300 text-sand-500',
    running: 'border-yellow-400/40 text-yellow-600',
    completed: 'border-emerald-400/40 text-emerald-600',
    failed: 'border-red-400/40 text-red-500',
    canceled: 'border-gray-300 text-sand-500',
  }
  const labels = { queued: '排队中', running: '生产中', completed: '已成功', failed: '失败', canceled: '已撤销' }
  return <span className={`rounded-lg border px-1.5 py-0.5 text-xs ${styles[status]}`}>{labels[status]}</span>
}

function subtitleASRTaskTime(task: SubtitleASRTask): string {
  const timestamp = task.completed_at || task.updated_at || task.started_at || task.created_at
  return timestamp > 0 ? new Date(timestamp * 1000).toLocaleString() : '-'
}

function asrProfileKey(profile: SubtitleASRProfile): string {
  return `${profile.provider}\n${profile.model}`
}

function errorMessage(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error
    || (error instanceof Error ? error.message : fallback)
}
