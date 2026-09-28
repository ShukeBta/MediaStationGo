import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Clock, Play } from 'lucide-react'

import { schedulerAPI, type JobStatus } from '../api/scheduler'

const jobLabels: Record<string, string> = {
  tmdb_episode_recheck: 'TMDb 季集复查（按播出时间分档）',
  tmdb_snapshot_backfill: 'TMDb 原始快照补全',
}

export function SchedulerPage() {
  const [jobs, setJobs] = useState<JobStatus[]>([])
  const [running, setRunning] = useState<string>('')

  const refresh = () => schedulerAPI.status().then(setJobs)
  useEffect(() => {
    refresh().catch(() => undefined)
    const id = window.setInterval(refresh, 5_000)
    return () => window.clearInterval(id)
  }, [])

  const runNow = async (name: string) => {
    setRunning(name)
    try {
      await schedulerAPI.run(name)
      toast.success(`${name} 已运行`)
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '运行失败'
      toast.error(msg)
    } finally {
      setRunning('')
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex items-center gap-3">
        <Clock className="h-6 w-6 text-brand-500" />
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">定时任务</h1>
          <p className="text-sm text-ink-50">
            后端周期性任务(媒体库扫描、转码缓存清理、回收站自动清理),每 5 秒刷新状态。
          </p>
        </div>
      </header>

      <div className="glass-panel">
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase tracking-wider text-sand-500">
            <tr>
              <th className="py-2">任务</th>
              <th>间隔</th>
              <th>上次运行</th>
              <th>错误</th>
              <th className="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((j) => (
              <tr key={j.name} className="border-t border-gray-200">
                <td className="py-2 text-ink-600">{jobLabels[j.name] || j.name}</td>
                <td className="text-ink-100">{j.interval}</td>
                <td className="text-ink-50">
                  {j.last_run && new Date(j.last_run).getFullYear() > 2000
                    ? new Date(j.last_run).toLocaleString()
                    : '尚未运行'}
                </td>
                <td className="text-red-400">{j.last_err || '—'}</td>
                <td className="py-2 text-right">
                  <button
                    onClick={() => runNow(j.name)}
                    disabled={running === j.name}
                    className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                  >
                    <Play size={12} className="inline" /> 立即运行
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
