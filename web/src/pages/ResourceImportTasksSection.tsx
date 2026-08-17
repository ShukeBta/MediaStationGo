import { useCallback, useEffect, useMemo, useState } from 'react'

import { resourceImportsAPI, type ResourceImportTask } from '../api/resourceImports'
import { confirmAction } from '../components/confirmAction'
import { Pagination } from '../components/Pagination'
import { ResourceImportTaskView } from './ResourceImportTaskView'
import {
  isResourceImportActive,
  mergeResourceImportTasks,
  resourceImportError,
} from './resourceImportModel'

export function ResourceImportTasksSection({ isAdmin, refreshKey = 0 }: { isAdmin: boolean; refreshKey?: number }) {
  const pageSize = 12
  const [tasks, setTasks] = useState<ResourceImportTask[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busyTaskID, setBusyTaskID] = useState('')
  const [busyAction, setBusyAction] = useState<'cancel' | 'retry' | 'delete' | null>(null)

  const load = useCallback(async () => {
    try {
      const incoming = await resourceImportsAPI.listAllPage(page, pageSize)
      setTasks(mergeResourceImportTasks([], incoming.items))
      setTotal(incoming.total)
      setError('')
    } catch (requestError) {
      setError(resourceImportError(requestError, '资源入库任务加载失败'))
    } finally {
      setLoading(false)
    }
  }, [page])

  useEffect(() => {
    let cancelled = false
    const tick = async () => {
      try {
        const incoming = await resourceImportsAPI.listAllPage(page, pageSize)
        if (cancelled) return
        setTasks(mergeResourceImportTasks([], incoming.items))
        setTotal(incoming.total)
        setError('')
      } catch (requestError) {
        if (!cancelled) setError(resourceImportError(requestError, '资源入库任务加载失败'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void tick()
    const timer = window.setInterval(tick, 3_000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [page, refreshKey])

  const activeTasks = useMemo(() => tasks.filter((task) => isResourceImportActive(task.status)), [tasks])
  const finishedTasks = useMemo(() => tasks.filter((task) => !isResourceImportActive(task.status)), [tasks])
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  useEffect(() => {
    if (page <= totalPages) return
    setPage(totalPages)
  }, [page, totalPages])

  const updateTask = (task: ResourceImportTask) => {
    setTasks((current) => mergeResourceImportTasks(current, [task]))
  }

  const cancelTask = async (task: ResourceImportTask) => {
    const confirmed = await confirmAction({
      title: '取消资源入库任务',
      message: '确定取消当前任务吗？取消后，已经转存到 115 的文件可能仍会保留。',
      confirmText: '取消任务',
      cancelText: '继续执行',
    })
    if (!confirmed) return
    setBusyTaskID(task.id)
    setBusyAction('cancel')
    setError('')
    try {
      updateTask(await resourceImportsAPI.cancel(task.id))
    } catch (requestError) {
      setError(resourceImportError(requestError, '取消任务失败'))
    } finally {
      setBusyTaskID('')
      setBusyAction(null)
    }
  }

  const retryTask = async (task: ResourceImportTask) => {
    setBusyTaskID(task.id)
    setBusyAction('retry')
    setError('')
    try {
      updateTask(await resourceImportsAPI.retry(task.id))
    } catch (requestError) {
      setError(resourceImportError(requestError, '重试任务失败'))
    } finally {
      setBusyTaskID('')
      setBusyAction(null)
    }
  }

  const deleteTask = async (task: ResourceImportTask) => {
    const confirmed = await confirmAction({
      title: '删除失败任务记录',
      message: `确定删除“${task.candidate_title || task.id}”的失败记录吗？此操作不会删除 115 上可能已经转存的文件。`,
      confirmText: '删除记录',
    })
    if (!confirmed) return
    setBusyTaskID(task.id)
    setBusyAction('delete')
    setError('')
    try {
      await resourceImportsAPI.removeFailed(task.id)
      setTasks((current) => current.filter((item) => item.id !== task.id))
      setTotal((current) => Math.max(0, current - 1))
    } catch (requestError) {
      setError(resourceImportError(requestError, '删除失败任务记录失败'))
    } finally {
      setBusyTaskID('')
      setBusyAction(null)
    }
  }

  return (
    <section className="glass-panel">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="font-display text-lg font-semibold text-ink-600">资源搜索入库任务</h2>
          <p className="mt-1 text-xs text-sand-500">
            {isAdmin ? '显示所有用户提交的任务' : '仅显示当前账号提交的任务'}
          </p>
        </div>
        <button type="button" className="btn-outline px-3 py-2" disabled={loading} onClick={() => void load()}>
          刷新
        </button>
      </div>

      {error && <p className="mb-3 break-words text-sm text-red-500">{error}</p>}
      {loading && tasks.length === 0 ? (
        <p className="text-sm text-sand-500">加载中…</p>
      ) : (
        <div className="space-y-5">
          <div>
            <h3 className="mb-1 text-sm font-semibold text-ink-500">运行中</h3>
            {activeTasks.length === 0 ? (
              <p className="py-3 text-sm text-sand-500">暂无运行中的资源入库任务。</p>
            ) : activeTasks.map((task) => (
              <ResourceImportTaskView
                key={task.id}
                task={task}
                showCreator={isAdmin}
                busyAction={busyTaskID === task.id ? busyAction : null}
                onCancel={(current) => void cancelTask(current)}
                onRetry={(current) => void retryTask(current)}
              />
            ))}
          </div>

          <div>
            <h3 className="mb-1 text-sm font-semibold text-ink-500">已结束</h3>
            {finishedTasks.length === 0 ? (
              <p className="py-3 text-sm text-sand-500">暂无已结束的资源入库任务。</p>
            ) : finishedTasks.map((task) => (
              <ResourceImportTaskView
                key={task.id}
                task={task}
                showCreator={isAdmin}
                busyAction={busyTaskID === task.id ? busyAction : null}
                onRetry={(current) => void retryTask(current)}
                onDelete={(current) => void deleteTask(current)}
              />
            ))}
          </div>

          {totalPages > 1 && (
            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} className="pt-1" />
          )}
        </div>
      )}
    </section>
  )
}
