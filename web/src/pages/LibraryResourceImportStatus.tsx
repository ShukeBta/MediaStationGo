import { CheckCircle2, X } from 'lucide-react'

import type { ResourceImportTask } from '../api/resourceImports'
import { ResourceImportTaskView } from './ResourceImportTaskView'

type LibraryResourceImportStatusProps = {
  activeTasks: ResourceImportTask[]
  latestCompletedTask: ResourceImportTask | null
  loading: boolean
  error: string
  onOpenTask: (task: ResourceImportTask) => void
  onDismissCompleted: () => void
  onRetryLoad: () => void
}

export function LibraryResourceImportStatus({
  activeTasks,
  latestCompletedTask,
  loading,
  error,
  onOpenTask,
  onDismissCompleted,
  onRetryLoad,
}: LibraryResourceImportStatusProps) {
  if (!loading && !error && activeTasks.length === 0 && !latestCompletedTask) return null

  return (
    <section className="border-y border-gray-200 bg-[var(--app-panel)] px-4 sm:px-5">
      {error && (
        <div className="flex flex-wrap items-center justify-between gap-2 py-3 text-sm text-red-500">
          <span>{error}</span>
          <button type="button" className="btn-outline px-3 py-2" onClick={onRetryLoad}>重新加载</button>
        </div>
      )}

      {!error && loading && activeTasks.length === 0 && (
        <p className="py-3 text-sm text-sand-500">正在加载资源入库任务…</p>
      )}

      {activeTasks.length > 0 && (
        <div className="py-2">
          <h2 className="pt-1 text-xs font-bold uppercase text-sand-500">本库正在入库</h2>
          {activeTasks.map((task) => (
            <ResourceImportTaskView key={task.id} task={task} compact onOpen={onOpenTask} />
          ))}
        </div>
      )}

      {latestCompletedTask && (
        <div className="flex min-w-0 items-start gap-3 border-t border-gray-200 py-3">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-ink-600">资源入库完成，媒体库已刷新</p>
            <ResourceImportTaskView task={latestCompletedTask} compact onOpen={onOpenTask} />
          </div>
          <button
            type="button"
            className="shrink-0 rounded-lg p-1.5 text-sand-500 hover:bg-gray-100 hover:text-ink-600"
            title="关闭完成提示"
            aria-label="关闭完成提示"
            onClick={onDismissCompleted}
          >
            <X size={16} />
          </button>
        </div>
      )}
    </section>
  )
}
