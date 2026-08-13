import { Link } from 'react-router-dom'
import {
  Ban,
  CheckCircle2,
  CircleAlert,
  Clock3,
  Film,
  LoaderCircle,
  RefreshCw,
  Trash2,
  UserRound,
} from 'lucide-react'

import type { ResourceImportTask } from '../api/resourceImports'
import {
  isResourceImportActive,
  isResourceImportCancelled,
  isResourceImportCompleted,
  isResourceImportCompletedWithWarning,
  isResourceImportFailed,
  resourceImportProgress,
  resourceImportTitle,
} from './resourceImportModel'

type ResourceImportTaskViewProps = {
  task: ResourceImportTask
  showCreator?: boolean
  compact?: boolean
  busyAction?: 'cancel' | 'retry' | 'delete' | null
  onOpen?: (task: ResourceImportTask) => void
  onCancel?: (task: ResourceImportTask) => void
  onRetry?: (task: ResourceImportTask) => void
  onDelete?: (task: ResourceImportTask) => void
}

export function ResourceImportTaskView({
  task,
  showCreator = false,
  compact = false,
  busyAction = null,
  onOpen,
  onCancel,
  onRetry,
  onDelete,
}: ResourceImportTaskViewProps) {
  const progress = resourceImportProgress(task.progress)
  const active = isResourceImportActive(task.status)
  const failed = isResourceImportFailed(task.status)
  const cancelled = isResourceImportCancelled(task.status)
  const completed = isResourceImportCompleted(task.status)
  const completedWithWarning = isResourceImportCompletedWithWarning(task.status)
  const canceling = task.status.toLowerCase() === 'canceling'
  const status = resourceImportStatus(task.status)
  const StatusIcon = status.icon

  return (
    <article className="min-w-0 border-b border-gray-200 py-3 last:border-b-0">
      <div className="flex min-w-0 items-start gap-3">
        <StatusIcon className={`mt-0.5 h-4 w-4 shrink-0 ${status.color} ${active ? 'animate-spin' : ''}`} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            {onOpen ? (
              <button
                type="button"
                className="min-w-0 truncate text-left text-sm font-semibold text-ink-600 hover:text-brand-500"
                title={resourceImportTitle(task)}
                onClick={() => onOpen(task)}
              >
                {resourceImportTitle(task)}
              </button>
            ) : (
              <h3 className="min-w-0 truncate text-sm font-semibold text-ink-600" title={resourceImportTitle(task)}>
                {resourceImportTitle(task)}
              </h3>
            )}
            <span className={`shrink-0 text-xs font-semibold ${status.color}`}>{status.label}</span>
          </div>

          {showCreator && (
            <div className="mt-1 flex min-w-0 items-center gap-1 text-xs text-sand-500">
              <UserRound size={12} className="shrink-0" />
              <span className="truncate">{task.creator_username || '未知用户'}</span>
              <span className="min-w-0 break-all font-mono">{task.user_id || '无用户 ID'}</span>
            </div>
          )}

          {task.stage && (
            <p className="mt-1 text-xs font-medium text-sand-500">阶段：{resourceImportStageLabel(task.stage)}</p>
          )}

          <p className={`mt-1 break-words text-xs ${failed ? 'text-red-500' : 'text-ink-50'} ${compact ? 'line-clamp-2' : ''}`}>
            {task.error || task.message || task.stage || '等待任务更新'}
          </p>

          {active && progress !== null && (
            <div className="mt-2 flex items-center gap-2">
              <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-gray-200">
                <div className="h-full rounded-full bg-brand-500 transition-[width]" style={{ width: `${progress}%` }} />
              </div>
              <span className="w-9 shrink-0 text-right text-xs font-medium text-sand-500">{progress}%</span>
            </div>
          )}

          {!compact && (
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-sand-500">
              {(task.library_name || task.library_id) && (
                <span className="max-w-full truncate">媒体库：{task.library_name || task.library_id}</span>
              )}
              {(task.root_name || task.root_id) && (
                <span className="max-w-full truncate">目录：{task.root_name || task.root_id}</span>
              )}
              {task.source && <span>来源：{task.source}</span>}
              {task.upgrade_media_id && (
                <span>类型：{task.upgrade_scope === 'work' ? '整剧升级' : '升级片源'} · {task.keep_old_version === false ? '替换旧版本' : '保留旧版本'}</span>
              )}
              <span>更新：{formatTaskTime(task.updated_at || task.created_at)}</span>
            </div>
          )}

          {(completed || failed || cancelled || onCancel) && (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {completed && task.media_id && (
                <Link className="btn-outline px-3 py-2" to={`/media/${task.media_id}`}>
                  <Film size={15} />
                  查看影片
                </Link>
              )}
              {active && !canceling && onCancel && (
                <button
                  type="button"
                  className="btn-outline px-3 py-2 text-red-500"
                  disabled={busyAction !== null}
                  onClick={() => onCancel(task)}
                >
                  <Ban size={15} />
                  {busyAction === 'cancel' ? '取消中…' : '取消任务'}
                </button>
              )}
              {(failed || cancelled || completedWithWarning) && onRetry && (
                <button
                  type="button"
                  className="btn-outline px-3 py-2"
                  disabled={busyAction !== null}
                  onClick={() => onRetry(task)}
                >
                  <RefreshCw size={15} />
                  {busyAction === 'retry' ? '重试中…' : completedWithWarning ? '重试告警阶段' : '重试'}
                </button>
              )}
              {failed && onDelete && (
                <button
                  type="button"
                  className="btn-outline px-3 py-2 text-red-500"
                  disabled={busyAction !== null}
                  onClick={() => onDelete(task)}
                >
                  <Trash2 size={15} />
                  {busyAction === 'delete' ? '删除中…' : '删除记录'}
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </article>
  )
}

function resourceImportStatus(status: string) {
  const normalized = status.toLowerCase()
  if (isResourceImportCompletedWithWarning(normalized)) {
    return { label: '已完成但有告警', color: 'text-amber-600', icon: CircleAlert }
  }
  if (isResourceImportCompleted(normalized)) {
    return { label: '已完成', color: 'text-emerald-500', icon: CheckCircle2 }
  }
  if (isResourceImportFailed(normalized)) {
    return { label: '失败', color: 'text-red-500', icon: CircleAlert }
  }
  if (isResourceImportCancelled(normalized)) {
    return { label: '已取消', color: 'text-sand-500', icon: Ban }
  }
  if (isResourceImportActive(normalized)) {
    return { label: activeStatusLabel(normalized), color: 'text-brand-500', icon: LoaderCircle }
  }
  return { label: status || '未知', color: 'text-sand-500', icon: Clock3 }
}

function activeStatusLabel(status: string): string {
  if (status === 'pending' || status === 'queued') return '排队中'
  if (status === 'retrying') return '重试中'
  if (status === 'canceling') return '取消中'
  return '进行中'
}

function resourceImportStageLabel(stage: string): string {
  const labels: Record<string, string> = {
    duplicate_check: '重复检查',
    submitting: '提交任务',
    transferring: '转存至 115',
    preparing_openlist: '准备 OpenList',
    scanning: '扫描媒体库',
    scraping: '刮削元数据',
    matching_subtitle: '匹配字幕',
    finalizing_upgrade: '处理旧版本',
    cleanup: '清理临时目录',
    completed: '完成',
  }
  return labels[stage] || stage
}

function formatTaskTime(value?: string): string {
  if (!value) return '-'
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? new Date(parsed).toLocaleString() : '-'
}
