import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Loader2,
  PackageCheck,
  RefreshCw,
  RotateCw,
  Server,
} from 'lucide-react'

import { adminAPI, type SystemUpdateStatus } from '../api/admin'

type UpdateErrorPayload = {
  response?: {
    data?: {
      error?: string
      status?: SystemUpdateStatus
    }
  }
}

export function SystemUpdatePanel() {
  const [status, setStatus] = useState<SystemUpdateStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [checking, setChecking] = useState(false)
  const [applying, setApplying] = useState(false)

  const refreshStatus = useCallback(async () => {
    const next = await adminAPI.systemUpdateStatus()
    setStatus(next)
    return next
  }, [])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    adminAPI
      .systemUpdateStatus()
      .then((next) => {
        if (!cancelled) setStatus(next)
      })
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!status?.running) return undefined
    const id = window.setInterval(() => {
      refreshStatus().catch(() => undefined)
    }, 3_000)
    return () => window.clearInterval(id)
  }, [refreshStatus, status?.running])

  const checkUpdate = async () => {
    setChecking(true)
    try {
      const next = await adminAPI.systemUpdateCheck()
      setStatus(next)
      toast.success(next.message || '检查完成')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '检查更新失败'))
    } finally {
      setChecking(false)
    }
  }

  const applyUpdate = async () => {
    setApplying(true)
    try {
      const next = await adminAPI.systemUpdateApply()
      setStatus(next)
      toast.success(next.message || '更新任务已启动')
    } catch (err: unknown) {
      const payload = err as UpdateErrorPayload
      if (payload.response?.data?.status) setStatus(payload.response.data.status)
      toast.error(apiErrorMessage(err, '启动更新失败'))
    } finally {
      setApplying(false)
    }
  }

  const tone = updateTone(status)
  const ToneIcon = tone.icon

  return (
    <section className="glass-panel space-y-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-gray-200 bg-white text-brand-500">
            <PackageCheck size={20} />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-display text-xl font-semibold text-ink-600">Docker Compose 一键更新</h2>
              <span className={`inline-flex items-center gap-1 rounded-lg border px-2 py-1 text-xs ${tone.className}`}>
                <ToneIcon size={13} />
                {tone.label}
              </span>
            </div>
            <p className="mt-1 text-sm text-ink-50">
              {loading ? '正在读取更新状态…' : status?.message || '尚未检查更新'}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            className="btn-outline"
            disabled={loading}
            onClick={() => refreshStatus().catch(() => toast.error('刷新状态失败'))}
            title="刷新状态"
          >
            {loading ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
            刷新
          </button>
          <button type="button" className="btn-outline" disabled={checking || status?.running} onClick={checkUpdate}>
            {checking ? <Loader2 size={16} className="animate-spin" /> : <CheckCircle2 size={16} />}
            检查更新
          </button>
          <button
            type="button"
            className="btn-primary"
            disabled={applying || status?.running}
            onClick={applyUpdate}
          >
            {applying || status?.running ? <Loader2 size={16} className="animate-spin" /> : <RotateCw size={16} />}
            一键更新
          </button>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <UpdateFact label="当前版本" value={status?.current_version || 'dev'} mono />
        <UpdateFact icon={Server} label="更新方式" value={status?.update_mode === 'compose' ? 'Docker Compose' : '自定义命令'} />
        <UpdateFact label="Compose 目录" value={status?.compose_dir || '自动识别'} mono />
        <UpdateFact label="应用镜像" value={status?.image || '-'} mono />
        <UpdateFact label="当前容器" value={status?.container_name || status?.container_id || '-'} mono />
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <DigestRow label="Compose 文件" digest={status?.compose_file} />
        <DigestRow label="Docker 连接" digest={status?.docker_available ? '可连接' : '未连接或未检查'} />
        <DigestRow label="本地摘要" digest={status?.local_digest} />
        <DigestRow label="远端摘要" digest={status?.remote_digest} />
      </div>

      {status?.task_id && (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50/70 px-3 py-2 text-sm">
          <span className="min-w-0 truncate text-ink-100">
            后台任务：<span className="font-mono">{status.task_id}</span>
          </span>
          <Link className="btn-ghost shrink-0 px-3 py-2" to="/tasks">
            查看任务
          </Link>
        </div>
      )}

      {status?.details && (
        <pre className="max-h-56 overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 font-mono text-xs leading-relaxed text-ink-100">
          {status.details}
        </pre>
      )}
    </section>
  )
}

function UpdateFact({
  icon: Icon,
  label,
  value,
  mono = false,
}: {
  icon?: typeof Server
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="rounded-xl border border-gray-200 bg-gray-50/60 px-3 py-2">
      <div className="flex items-center gap-1.5 text-xs text-sand-500">
        {Icon && <Icon size={13} />}
        {label}
      </div>
      <div className={`mt-1 truncate text-sm text-ink-600 ${mono ? 'font-mono' : ''}`} title={value}>
        {value}
      </div>
    </div>
  )
}

function DigestRow({ label, digest }: { label: string; digest?: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50/60 px-3 py-2">
      <span className="shrink-0 text-xs text-sand-500">{label}</span>
      <span className="min-w-0 truncate font-mono text-xs text-ink-100" title={digest || ''}>
        {shortDigest(digest)}
      </span>
    </div>
  )
}

function updateTone(status: SystemUpdateStatus | null): {
  label: string
  className: string
  icon: typeof Clock3
} {
  if (!status) {
    return { label: '读取中', className: 'border-gray-200 text-sand-500', icon: Clock3 }
  }
  if (status.running) {
    return { label: '更新中', className: 'border-yellow-400/50 text-yellow-600', icon: Loader2 }
  }
  if (!status.can_apply && status.message && status.message !== '尚未检查更新') {
    return { label: '不可更新', className: 'border-red-400/50 text-red-500', icon: AlertTriangle }
  }
  if (status.update_available === true) {
    return { label: '有新版', className: 'border-emerald-400/50 text-emerald-600', icon: CheckCircle2 }
  }
  if (status.update_available === false) {
    return { label: '已最新', className: 'border-gray-300 text-ink-100', icon: CheckCircle2 }
  }
  return { label: '可执行', className: 'border-brand-300/60 text-brand-500', icon: CheckCircle2 }
}

function shortDigest(digest?: string): string {
  if (!digest) return '-'
  return digest.length > 24 ? `${digest.slice(0, 18)}…${digest.slice(-8)}` : digest
}

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as UpdateErrorPayload).response?.data?.error || fallback
}
