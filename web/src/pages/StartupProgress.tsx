import { useEffect, useState } from 'react'
import { tasksAPI, type StartupStatus } from '../api/tasks'

export function useStartupStatus() {
  const [status, setStatus] = useState<StartupStatus | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    const refresh = async () => {
      try {
        const next = await tasksAPI.startup()
        if (active) { setStatus(next); setError('') }
      } catch { if (active) setError('启动进度暂时无法读取，正在重试') }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])
  return { status, error }
}

export function StartupProgress({ status, error }: ReturnType<typeof useStartupStatus>) {
  if (status?.state === 'ready' && !status.warnings.length && !error) return null
  return <section className="rounded-xl border border-gray-200 bg-white p-4" aria-live="polite">
    <h2 className="font-semibold text-ink-600">{status?.state === 'failed' ? '后台初始化未完成' : status?.state === 'ready' ? '初始化完成，部分服务需要检查' : '后台初始化中'}</h2>
    {error && <p className="mt-1 text-sm text-amber-600">{error}</p>}
    {status && <>
      <p className="mt-1 text-sm text-ink-100">{status.stage} · 阶段 {status.stage_elapsed_seconds} 秒 · 累计 {status.elapsed_seconds} 秒</p>
      <p className="mt-1 text-sm text-sand-500">已发现 {status.directories_found} 个目录，已监听 {status.directories_watched} 个目录</p>
      {status.warnings.map((warning, index) => <p key={index} className="mt-1 text-sm text-amber-600">{warning}</p>)}
    </>}
    {status?.state === 'starting' && <p className="mt-1 text-sm text-sand-500">初始化完成后即可手动执行扫描和定时任务。</p>}
  </section>
}

export function StartupProgressPanel() {
  return <StartupProgress {...useStartupStatus()} />
}
