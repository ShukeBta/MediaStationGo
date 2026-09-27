import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { resourceImportsAPI, type ResourceImportTask } from '../api/resourceImports'
import {
  isResourceImportActive,
  isResourceImportCompleted,
  mergeResourceImportTasks,
  resourceImportError,
} from './resourceImportModel'

export function useLibraryResourceImports(libraryID: string, userID: string, onLibraryChanged: () => void) {
  const [tasks, setTasks] = useState<ResourceImportTask[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [highlightedMediaID, setHighlightedMediaID] = useState('')
  const [latestCompletedTask, setLatestCompletedTask] = useState<ResourceImportTask | null>(null)
  const initializedRef = useRef(false)
  const handledCompletedRef = useRef(new Set<string>())

  const handleCompletedTasks = useCallback((incoming: ResourceImportTask[], initial: boolean) => {
    for (const task of incoming) {
      if (!isResourceImportCompleted(task.status) || handledCompletedRef.current.has(task.id)) continue
      handledCompletedRef.current.add(task.id)
      if (initial) continue
      setLatestCompletedTask(task)
      if (task.media_id) setHighlightedMediaID(task.media_id)
      onLibraryChanged()
    }
  }, [onLibraryChanged])

  const acceptTasks = useCallback((incoming: ResourceImportTask[], replace = false) => {
    const initial = !initializedRef.current
    handleCompletedTasks(incoming, initial)
    initializedRef.current = true
    setTasks((current) => replace
      ? mergeResourceImportTasks([], incoming)
      : mergeResourceImportTasks(current, incoming))
  }, [handleCompletedTasks])

  const refresh = useCallback(async () => {
    if (!libraryID) return
    try {
      const incoming = currentUserTasks(await resourceImportsAPI.listLibrary(libraryID), userID)
      acceptTasks(incoming, true)
      setError('')
    } catch (requestError) {
      setError(resourceImportError(requestError, '资源入库任务加载失败'))
    } finally {
      setLoading(false)
    }
  }, [acceptTasks, libraryID, userID])

  useEffect(() => {
    initializedRef.current = false
    handledCompletedRef.current = new Set()
    setTasks([])
    setError('')
    setLoading(true)
    setHighlightedMediaID('')
    setLatestCompletedTask(null)
    if (!libraryID) return

    let cancelled = false
    const tick = async () => {
      try {
        const incoming = currentUserTasks(await resourceImportsAPI.listLibrary(libraryID), userID)
        if (cancelled) return
        acceptTasks(incoming, true)
        setError('')
      } catch (requestError) {
        if (!cancelled) setError(resourceImportError(requestError, '资源入库任务加载失败'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void tick()
    return () => {
      cancelled = true
    }
  }, [acceptTasks, libraryID, userID])

  const activeTasks = useMemo(
    () => tasks.filter((task) => isResourceImportActive(task.status)),
    [tasks],
  )

  useEffect(() => {
    if (!libraryID || activeTasks.length === 0) return
    let cancelled = false
    let polling = false

    const tick = async () => {
      if (cancelled || polling || document.visibilityState === 'hidden') return
      polling = true
      try {
        const incoming = currentUserTasks(await resourceImportsAPI.listLibrary(libraryID), userID)
        if (cancelled) return
        acceptTasks(incoming, true)
        setError('')
      } catch (requestError) {
        if (!cancelled) setError(resourceImportError(requestError, '资源入库任务加载失败'))
      } finally {
        polling = false
      }
    }
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') void tick()
    }
    const timer = window.setInterval(() => void tick(), 3_000)
    document.addEventListener('visibilitychange', onVisibilityChange)
    return () => {
      cancelled = true
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [acceptTasks, activeTasks.length, libraryID, userID])

  const acceptTask = useCallback((task: ResourceImportTask) => acceptTasks([task]), [acceptTasks])
  const dismissCompletedTask = useCallback(() => setLatestCompletedTask(null), [])

  return {
    tasks,
    activeTasks,
    loading,
    error,
    highlightedMediaID,
    latestCompletedTask,
    acceptTask,
    refresh,
    dismissCompletedTask,
  }
}

function currentUserTasks(tasks: ResourceImportTask[], userID: string): ResourceImportTask[] {
  if (!userID) return tasks
  return tasks.filter((task) => !task.user_id || task.user_id === userID)
}
