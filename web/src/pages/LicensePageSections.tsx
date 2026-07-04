import type { FormEvent } from 'react'
import {
  CheckCircle2,
  KeySquare,
  Link2,
  Loader2,
  RefreshCw,
  XCircle,
} from 'lucide-react'

import type { LicenseStatus } from '../api/license'

export function LicenseHeader() {
  return (
    <div className="flex items-center gap-3">
      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-fuchsia-400/10 text-fuchsia-300">
        <KeySquare size={20} />
      </div>
      <div>
        <h1 className="font-display text-3xl font-bold text-ink-600">许可证</h1>
        <p className="text-sm text-ink-50">绑定授权密钥以提升多用户容量</p>
      </div>
    </div>
  )
}

export function LicenseBindPanel({
  bindKey,
  binding,
  onBind,
  onBindKeyChange,
}: {
  bindKey: string
  binding: boolean
  onBind: (event: FormEvent<HTMLFormElement>) => void
  onBindKeyChange: (value: string) => void
}) {
  return (
    <div className="glass-panel space-y-4">
      <div className="flex items-center gap-2">
        <Link2 size={18} className="text-brand-500" />
        <h2 className="font-display text-lg font-semibold text-ink-600">绑定许可证</h2>
      </div>
      <form onSubmit={onBind} className="flex gap-3">
        <input
          className="input-base flex-1 font-mono text-sm tracking-wider"
          placeholder="MS-XXXX-XXXX-XXXX"
          value={bindKey}
          onChange={(event) => onBindKeyChange(event.target.value)}
          disabled={binding}
        />
        <button type="submit" disabled={binding} className="neon-button shrink-0">
          {binding ? <Loader2 size={16} className="animate-spin" /> : <Link2 size={16} />}
          绑定
        </button>
      </form>
      <p className="text-xs text-sand-500">
        输入从授权服务器获取的许可证密钥，激活后按授权额度开放更多平台用户。
      </p>
    </div>
  )
}

export function LicenseStatusPanel({
  status,
  loadingStatus,
  active,
  isExpired,
  onRefresh,
  onHeartbeat,
}: {
  status: LicenseStatus | null
  loadingStatus: boolean
  active: boolean
  isExpired: boolean
  onRefresh: () => void
  onHeartbeat: () => void
}) {
  const activation = status?.activation

  if (loadingStatus) {
    return (
      <div className="flex justify-center py-8 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  return (
    <div className="glass-panel space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {active && !isExpired ? (
            <CheckCircle2 size={18} className="text-emerald-400" />
          ) : (
            <XCircle size={18} className="text-sand-500" />
          )}
          <h2 className="font-display text-lg font-semibold text-ink-600">当前状态</h2>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            onClick={onRefresh}
            className="rounded-lg border border-gray-200 p-2 text-ink-50 hover:bg-gray-50 hover:text-white"
            title="刷新状态"
          >
            <RefreshCw size={14} />
          </button>
          {active && (
            <button
              onClick={onHeartbeat}
              className="rounded-lg border border-primary-400/30 px-3 py-1.5 text-xs text-brand-500 hover:bg-primary-400/10"
            >
              心跳上报
            </button>
          )}
        </div>
      </div>

      {!active && (
        <div className="space-y-3 rounded-xl border border-gray-200 bg-gray-50 p-4 text-center">
          <p className="text-ink-50">
            尚未绑定许可证。请在上方输入密钥完成激活。
          </p>
        </div>
      )}

      {active && activation && (
        <div className="grid grid-cols-2 gap-3">
          <StatusBadge label="密钥" value={activation.key || activation.key_id} mono />
          <StatusBadge
            label="套餐"
            value={activation.plan ?? 'standard'}
            className={
              activation.plan === 'enterprise'
                ? 'text-amber-300'
                : activation.plan === 'pro'
                  ? 'text-brand-500'
                  : 'text-ink-100'
            }
          />
          <StatusBadge label="设备名称" value={activation.device_name || 'Web Client'} />
          <StatusBadge label="设备数" value={`${activation.max_activations ?? 1} 台`} />
          <StatusBadge label="绑定时间" value={fmtDate(activation.created_at)} />
          <StatusBadge
            label="到期时间"
            value={fmtDate(activation.expires_at)}
            className={isExpired ? 'text-red-400' : undefined}
          />
          <StatusBadge label="最近心跳" value={fmtDateTime(activation.heartbeat_at)} />
          <StatusBadge label="客户端 IP" value={activation.ip ?? '—'} />
          <StatusBadge
            label="用户额度"
            value={fmtUserLimit(status?.max_users, status?.unlimited_users)}
          />
        </div>
      )}

      {isExpired && (
        <div className="rounded-xl border border-red-400/20 bg-red-400/5 px-4 py-3 text-sm text-red-400">
          此许可证已过期，部分功能可能受限。请获取新的许可证密钥。
        </div>
      )}
    </div>
  )
}

export function LicenseInactiveTip({ active, loadingStatus }: { active: boolean; loadingStatus: boolean }) {
  if (active || loadingStatus) return null
  return (
    <div className="rounded-xl border border-gray-200 bg-gray-50 p-5 text-center">
      <p className="text-sm text-sand-500">
        需要获取许可证？请联系管理员获取 MediaStationGo 授权密钥。
      </p>
      <p className="mt-1 text-xs text-gray-500">
        授权服务器地址可在系统设置中配置
      </p>
    </div>
  )
}

function StatusBadge({
  label,
  value,
  mono,
  className,
}: {
  label: string
  value: string
  mono?: boolean
  className?: string
}) {
  return (
    <div className="rounded-xl border border-gray-200 bg-gray-50 p-3">
      <p className="mb-0.5 text-xs text-sand-500">{label}</p>
      <p className={`text-sm text-ink-600 ${mono ? 'font-mono' : ''} ${className ?? ''}`}>
        {value}
      </p>
    </div>
  )
}

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return '永久'
  return new Date(iso).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
}

function fmtDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-CN')
}

function fmtUserLimit(maxUsers: number | null | undefined, unlimited?: boolean): string {
  if (unlimited || maxUsers == null) return '不限制'
  return `${maxUsers} 人`
}
