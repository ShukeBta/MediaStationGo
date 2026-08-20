import { useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Activity, Copy, RotateCw, SlidersHorizontal } from 'lucide-react'

import { toolsAPI } from '../api/tools'
import { libraryAPI } from '../api/library'
import { tasksAPI, type BackgroundTask, type TaskItem, type TasksSnapshot } from '../api/tasks'
import { useWebSocket } from '../hooks/useWebSocket'
import { confirmAction } from '../components/confirmAction'
import { TorrentTaskTable, TranscodeTaskTable } from './TaskRuntimeTables'
import { ItemRetryDialog } from './TaskItemRetryDialog'

const metricLabels: Record<string, string> = {
  organized: '新增',
  replaced: '替换',
  reclassified: '纠偏',
  skipped: '跳过',
  errors: '错误',
  scans: '扫描库',
  scan_visited: '访问',
  scan_added: '入库',
  scan_updated: '更新',
  scan_removed: '移除',
  scan_errors: '扫描错误',
  scrapes: '刮削库',
  scrape_matched: '匹配',
  scrape_processed: '刮削处理',
  scrape_skipped: '刮削跳过',
  scrape_errors: '刮削错误',
  skip_already_organized: '已在目标',
  skip_duplicate_in_library: '已入库去重',
  skip_target_file_exists: '目标已存在',
  skip_sample_trailer_clip: '样片过滤',
  skip_duplicate_exists: '重复跳过',
  skip_target_exists: '目标已存在',
  visited: '访问',
  added: '入库',
  updated: '更新',
  probed: '探测',
  local_metadata: '本地元数据',
  removed: '移除',
  matched: '匹配',
  processed: '处理',
  queued: '排队',
}

function formatMetrics(metrics?: Record<string, number>): string {
  if (!metrics) return ''
  return Object.entries(metrics)
    .filter(([, value]) => Number.isFinite(value) && value !== 0)
    .map(([key, value]) => `${metricLabels[key] ?? key} ${value}`)
    .join(' · ')
}

function hasTaskIssues(task: BackgroundTask): boolean {
  return Boolean(task.metrics?.errors || task.metrics?.scan_errors || task.metrics?.scrape_errors)
}

const itemKindLabels: Record<string, string> = {
  organize: '整理/重命名',
  scan: '入库',
  scrape: '刮削',
}

const itemStatusLabels: Record<string, string> = {
  pending: '待进行',
  running: '进行中',
  succeeded: '成功',
  failed: '失败',
}

function statusBadge(task: BackgroundTask) {
  if (task.status === 'failed') {
    return <span className="rounded-lg border border-red-400/40 px-1.5 py-0.5 text-xs text-red-500">failed</span>
  }
  if (hasTaskIssues(task)) {
    return <span className="rounded-lg border border-orange-400/40 px-1.5 py-0.5 text-xs text-orange-500">issues</span>
  }
  if (task.status === 'completed') {
    return <span className="rounded-lg border border-emerald-400/40 px-1.5 py-0.5 text-xs text-emerald-500">done</span>
  }
  return <span className="rounded-lg border border-yellow-400/40 px-1.5 py-0.5 text-xs text-yellow-500">running</span>
}

function itemStatusBadge(item: TaskItem) {
  switch (item.status) {
    case 'failed':
      return <span className="rounded-lg border border-red-400/40 px-1.5 py-0.5 text-xs text-red-500">失败</span>
    case 'running':
      return <span className="rounded-lg border border-yellow-400/40 px-1.5 py-0.5 text-xs text-yellow-500">进行中</span>
    case 'succeeded':
      return <span className="rounded-lg border border-emerald-400/40 px-1.5 py-0.5 text-xs text-emerald-500">成功</span>
    default:
      return <span className="rounded-lg border border-gray-300 px-1.5 py-0.5 text-xs text-sand-500">待进行</span>
  }
}

function taskCopyText(task: BackgroundTask): string {
  const lines = [
    `任务: ${task.name}`,
    `状态: ${task.status}${hasTaskIssues(task) ? ' (issues)' : ''}`,
    `阶段: ${task.stage || '-'}`,
    `来源: ${task.source_path || '-'}`,
    `目标: ${task.dest_path || '-'}`,
    `消息: ${task.error || task.message || '-'}`,
  ]
  const metrics = formatMetrics(task.metrics)
  if (metrics) lines.push(`指标: ${metrics}`)
  if (task.details?.length) {
    lines.push('详情:')
    lines.push(...task.details)
  }
  if (task.items?.length) {
    lines.push('条目:')
    lines.push(
      ...task.items.map(
        (item) =>
          `[${itemKindLabels[item.kind] ?? item.kind}] ${item.name} → ${itemStatusLabels[item.status] ?? item.status}${
            item.error ? ` (${item.error})` : ''
          }`,
      ),
    )
  }
  return lines.join('\n')
}

async function copyTask(task: BackgroundTask) {
  try {
    await navigator.clipboard.writeText(taskCopyText(task))
    toast.success('任务详情已复制')
  } catch {
    toast.error('复制失败，请手动选中详情文本')
  }
}

// Renders one task's per-item rows, each with status + (for failed items)
// retry / manual-handle actions.
function TaskItemRows({
  task,
  onRetry,
  onManual,
}: {
  task: BackgroundTask
  onRetry: (item: TaskItem) => void
  onManual: (item: TaskItem) => void
}) {
  const items = task.items ?? []
  if (items.length === 0) {
    // Fall back to the compact aggregate view for legacy tasks without items.
    return (
      <div className="flex items-center justify-between gap-3 py-1">
        <div className="min-w-0">
          <div className="truncate font-medium text-ink-600" title={task.source_path || task.dest_path}>
            {task.name}
          </div>
          <div className="truncate font-mono text-xs text-sand-500">
            {task.source_path || task.dest_path || task.message || '-'}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {statusBadge(task)}
          {hasTaskIssues(task) && (
            <button
              type="button"
              className="rounded-lg border border-gray-200 bg-white p-1 text-sand-500 hover:border-primary-400/40 hover:text-brand-500"
              title="复制任务详情"
              onClick={() => void copyTask(task)}
            >
              <Copy size={14} />
            </button>
          )}
        </div>
      </div>
    )
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="py-1">名称</th>
            <th>类型</th>
            <th>状态</th>
            <th>路径</th>
            <th className="text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id} className="border-t border-gray-100 align-top">
              <td className="max-w-xs py-1.5">
                <div className="truncate font-medium text-ink-600" title={item.name}>
                  {item.name}
                </div>
                {item.error && (
                  <div className="truncate font-mono text-[11px] text-red-500" title={item.error}>
                    {item.error}
                  </div>
                )}
              </td>
              <td className="py-1.5">
                <span className="rounded-md bg-gray-100 px-1.5 py-0.5 text-xs text-sand-600">
                  {itemKindLabels[item.kind] ?? item.kind}
                </span>
              </td>
              <td className="py-1.5">{itemStatusBadge(item)}</td>
              <td className="max-w-sm py-1.5">
                <div className="truncate font-mono text-xs text-sand-500" title={item.source || item.dest_path}>
                  {item.source || item.dest_path || '-'}
                </div>
              </td>
              <td className="py-1.5 text-right whitespace-nowrap">
                {item.status === 'failed' && (
                  <div className="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-white px-2 py-1 text-xs text-ink-600 hover:border-primary-400/40 hover:text-brand-500"
                      title="一键重试该条目"
                      onClick={() => onRetry(item)}
                    >
                      <RotateCw size={12} />
                      重试
                    </button>
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-white px-2 py-1 text-xs text-ink-600 hover:border-primary-400/40 hover:text-brand-500"
                      title="打开手动处理表单"
                      onClick={() => onManual(item)}
                    >
                      <SlidersHorizontal size={12} />
                      手动处理
                    </button>
                  </div>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function TaskSection({
  tasks,
  empty,
  onRetry,
  onManual,
}: {
  tasks: BackgroundTask[]
  empty: string
  onRetry: (task: BackgroundTask, item: TaskItem) => void
  onManual: (task: BackgroundTask, item: TaskItem) => void
}) {
  const withItems = tasks.filter((task) => (task.items ?? []).length > 0)
  const withoutItems = tasks.filter((task) => (task.items ?? []).length === 0)
  if (tasks.length === 0) return <p className="text-sand-500">{empty}</p>
  return (
    <div className="space-y-4">
      {withItems.length > 0 && (
        <div className="space-y-3">
          {withItems.map((task) => (
            <div key={task.id} className="rounded-xl border border-gray-200 bg-gray-50/60 p-3">
              <div className="mb-1 flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-medium text-ink-600">{task.name}</span>
                  {statusBadge(task)}
                </div>
                <button
                  type="button"
                  className="rounded-lg border border-gray-200 bg-white p-1 text-sand-500 hover:border-primary-400/40 hover:text-brand-500"
                  title="复制任务详情"
                  onClick={() => void copyTask(task)}
                >
                  <Copy size={13} />
                </button>
              </div>
              <TaskItemRows task={task} onRetry={(item) => onRetry(task, item)} onManual={(item) => onManual(task, item)} />
              {formatMetrics(task.metrics) && (
                <div className="mt-2 select-text text-xs text-sand-500">{formatMetrics(task.metrics)}</div>
              )}
            </div>
          ))}
        </div>
      )}
      {withoutItems.length > 0 && (
        <div className="space-y-2">
          {withoutItems.map((task) => (
            <div key={task.id} className="rounded-xl border border-gray-200 bg-gray-50/60 p-3">
              <TaskItemRows
                task={task}
                onRetry={(item) => onRetry(task, item)}
                onManual={(item) => onManual(task, item)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// TasksPage shows everything the backend is doing right now: ffmpeg
// transcodes + qBittorrent downloads + item-level organize/ingest/scrape
// progress. Refreshes every 3 s and reconciles against live WS "task" events.
export function TasksPage() {
  const [snap, setSnap] = useState<TasksSnapshot | null>(null)
  const [manualTarget, setManualTarget] = useState<{ task: BackgroundTask; item: TaskItem } | null>(null)

  const mergeEvent = useCallback((payload: unknown) => {
    const eventTask = payload as BackgroundTask
    if (!eventTask || typeof eventTask.id !== 'string') return
    setSnap((prev) => {
      if (!prev) return prev
      const merge = (list: BackgroundTask[]) =>
        list.map((t) => (t.id === eventTask.id ? { ...eventTask } : t))
      return {
        ...prev,
        background_tasks: {
          active: merge(prev.background_tasks?.active ?? []),
          recent: merge(prev.background_tasks?.recent ?? []),
        },
      }
    })
  }, [])

  useWebSocket((topic, payload) => {
    if (topic === 'task') mergeEvent(payload)
  })

  useEffect(() => {
    let cancelled = false
    const tick = () =>
      tasksAPI.snapshot().then((s) => {
        if (!cancelled) setSnap(s)
      })
    void tick()
    const id = window.setInterval(tick, 3_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [])

  if (!snap) return <p className="text-sand-500">加载中…</p>

  const torrents = snap.torrents ?? []
  const background = snap.background_tasks ?? { active: [], recent: [] }

  const handleRetry = async (task: BackgroundTask, item: TaskItem) => {
    const ok = await confirmAction({
      title: '重试失败条目',
      message: `确定要重新处理「${item.name}」吗？\n类型：${itemKindLabels[item.kind] ?? item.kind}`,
      confirmText: '重试',
    })
    if (!ok) return
    try {
      if (item.kind === 'scan' && item.library_id) {
        await libraryAPI.scan(item.library_id)
        toast.success(`已重新入库：${item.name}`)
      } else if (item.kind === 'scrape' && item.library_id) {
        await libraryAPI.scrape(item.library_id)
        toast.success(`已重新刮削：${item.name}`)
      } else if (item.kind === 'organize' && item.source) {
        const dest = task.dest_path || item.dest_path || undefined
        await toolsAPI.organizeDirectory({
          source_path: item.source,
          dest_path: dest,
          scan_after: true,
          scrape_after: true,
        })
        toast.success(`已重新整理：${item.name}`)
      } else {
        toast.error(`无法重试：缺少必要信息（${item.kind}）`)
        return
      }
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '重试失败')
    }
  }

  const handleManual = (task: BackgroundTask, item: TaskItem) => {
    setManualTarget({ task, item })
  }

  return (
    <div className="space-y-8">
      <header className="flex items-center gap-3">
        <Activity className="h-6 w-6 text-brand-500" />
        <h1 className="font-display text-3xl font-bold text-ink-600">实时任务</h1>
      </header>

      <section className="glass-panel">
        <h2 className="mb-3 font-display text-lg font-semibold text-ink-600">整理 / 重命名 / 入库 / 刮削任务</h2>
        <div className="space-y-5">
          <div>
            <h3 className="mb-2 text-sm font-semibold text-ink-500">运行中</h3>
            <TaskSection
              tasks={background.active}
              empty="暂无运行中的整理、重命名、入库或刮削任务。"
              onRetry={(task, item) => void handleRetry(task, item)}
              onManual={handleManual}
            />
          </div>
          <div>
            <h3 className="mb-2 text-sm font-semibold text-ink-500">最近完成</h3>
            <TaskSection
              tasks={background.recent.slice(0, 10)}
              empty="暂无最近完成的后台任务。"
              onRetry={(task, item) => void handleRetry(task, item)}
              onManual={handleManual}
            />
          </div>
        </div>
      </section>

      <section className="glass-panel">
        <h2 className="mb-3 font-display text-lg font-semibold text-ink-600">转码任务</h2>
        <TranscodeTaskTable transcodes={snap.transcodes} />
      </section>

      <section className="glass-panel">
        <h2 className="mb-3 font-display text-lg font-semibold text-ink-600">下载任务</h2>
        <TorrentTaskTable torrents={torrents} />
      </section>

      {manualTarget && (
        <ItemRetryDialog
          task={manualTarget.task}
          item={manualTarget.item}
          onClose={() => setManualTarget(null)}
        />
      )}
    </div>
  )
}
