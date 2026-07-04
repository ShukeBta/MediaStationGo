import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Activity, Copy } from 'lucide-react'

import { tasksAPI, type BackgroundTask, type TasksSnapshot } from '../api/tasks'
import { TorrentTaskTable, TranscodeTaskTable } from './TaskRuntimeTables'

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

function BackgroundTaskTable({ tasks, empty }: { tasks: BackgroundTask[]; empty: string }) {
  if (tasks.length === 0) return <p className="text-sand-500">{empty}</p>
  return (
    <table className="w-full text-left text-sm">
      <thead className="text-xs uppercase tracking-wider text-sand-500">
        <tr>
          <th className="py-2">任务</th>
          <th>阶段</th>
          <th>状态</th>
          <th>结果</th>
          <th>时间</th>
        </tr>
      </thead>
      <tbody>
        {tasks.map((task) => (
          <tr key={task.id} className="border-t border-gray-200 align-top">
            <td className="max-w-md py-2">
              <div className="font-medium text-ink-600">{task.name}</div>
              <div className="truncate font-mono text-xs text-sand-500" title={task.source_path || task.dest_path}>
                {task.source_path || task.dest_path || task.message || '-'}
              </div>
            </td>
            <td className="text-ink-100">{task.stage || '-'}</td>
            <td>{statusBadge(task)}</td>
            <td className="max-w-md text-ink-100">
              <div className="flex items-start gap-2">
                <div className="min-w-0 flex-1 select-text break-words">{task.error || task.message || '-'}</div>
                <button
                  type="button"
                  className="rounded-lg border border-gray-200 bg-white p-1 text-sand-500 hover:border-primary-400/40 hover:text-brand-500"
                  title="复制任务详情"
                  onClick={() => void copyTask(task)}
                >
                  <Copy size={14} />
                </button>
              </div>
              {formatMetrics(task.metrics) && (
                <div className="mt-1 select-text text-xs text-sand-500">{formatMetrics(task.metrics)}</div>
              )}
              {task.details && task.details.length > 0 && (
                <div className="mt-2 max-h-64 select-text overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 font-mono text-[11px] leading-relaxed text-sand-600">
                  {task.details.map((line, index) => (
                    <div key={`${task.id}-detail-${index}`} className="whitespace-pre-wrap break-words">
                      {line}
                    </div>
                  ))}
                </div>
              )}
            </td>
            <td className="text-ink-100">
              {new Date(task.finished_at || task.updated_at || task.started_at).toLocaleTimeString()}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// TasksPage shows everything the backend is doing right now: ffmpeg
// transcodes + qBittorrent downloads. Refreshes every 3 s.
export function TasksPage() {
  const [snap, setSnap] = useState<TasksSnapshot | null>(null)

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
            <BackgroundTaskTable tasks={background.active} empty="暂无运行中的整理、重命名、入库或刮削任务。" />
          </div>
          <div>
            <h3 className="mb-2 text-sm font-semibold text-ink-500">最近完成</h3>
            <BackgroundTaskTable tasks={background.recent.slice(0, 10)} empty="暂无最近完成的后台任务。" />
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
    </div>
  )
}
