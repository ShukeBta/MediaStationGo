import { FormEvent } from 'react'
import { Plus } from 'lucide-react'

type AdminUsersFormProps = {
  usersCount: number
  userLimitLabel: string
  username: string
  password: string
  userLimitReached: boolean
  onUsernameChange: (value: string) => void
  onPasswordChange: (value: string) => void
  onSubmit: (e: FormEvent) => void
}

export function AdminUsersForm({
  usersCount,
  userLimitLabel,
  username,
  password,
  userLimitReached,
  onUsernameChange,
  onPasswordChange,
  onSubmit,
}: AdminUsersFormProps) {
  return (
    <form onSubmit={onSubmit} className="glass-panel grid gap-3 md:grid-cols-[1fr_1fr_auto]">
      <div className="md:col-span-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-display text-lg font-semibold text-ink-600">用户管理</h2>
          <p className="text-xs text-sand-500">
            已创建 {usersCount}/{userLimitLabel} 个用户；新增用户默认只有媒体库浏览、播放、外部播放器与第三方客户端观看权限。
          </p>
        </div>
        <span className="rounded-full border border-primary-400/30 px-3 py-1 text-xs text-brand-500">
          默认管理员不可删除 · 最高权限
        </span>
      </div>
      <input
        required
        className="input-base"
        placeholder="用户名"
        value={username}
        onChange={(e) => onUsernameChange(e.target.value)}
        disabled={userLimitReached}
      />
      <input
        required
        minLength={6}
        className="input-base"
        placeholder="初始密码（至少 6 位）"
        type="password"
        value={password}
        onChange={(e) => onPasswordChange(e.target.value)}
        disabled={userLimitReached}
      />
      <button type="submit" className="neon-button inline-flex items-center justify-center gap-2" disabled={userLimitReached}>
        <Plus size={16} />
        添加用户
      </button>
    </form>
  )
}
