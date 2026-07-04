import { KeyRound, Loader2, Pencil, ShieldCheck, Trash2, UserCheck, UserX, X } from 'lucide-react'

import type { User } from '../types'

type AdminUsersTableProps = {
  users: User[]
  editingID: string | null
  editingUsername: string
  resettingPasswordID: string | null
  onEditingUsernameChange: (value: string) => void
  onSaveEdit: (id: string) => void
  onCancelEdit: () => void
  onStartEdit: (user: User) => void
  onResetPassword: (user: User) => void
  onToggleStatus: (user: User) => void
  onDeleteUser: (user: User) => void
}

export function AdminUsersTable({
  users,
  editingID,
  editingUsername,
  resettingPasswordID,
  onEditingUsernameChange,
  onSaveEdit,
  onCancelEdit,
  onStartEdit,
  onResetPassword,
  onToggleStatus,
  onDeleteUser,
}: AdminUsersTableProps) {
  return (
    <div className="glass-panel overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="py-2">用户名</th>
            <th>角色</th>
            <th>状态</th>
            <th>权限说明</th>
            <th>最近登录</th>
            <th className="text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id} className="border-t border-gray-200">
              <td className="py-2 text-ink-600">
                {editingID === u.id ? (
                  <input
                    className="input-base h-9 max-w-48"
                    value={editingUsername}
                    onChange={(e) => onEditingUsernameChange(e.target.value)}
                  />
                ) : (
                  <span className="inline-flex items-center gap-2">
                    {u.username}
                    {u.is_default_admin && <ShieldCheck size={15} className="text-brand-500" />}
                  </span>
                )}
              </td>
              <td className="text-ink-100">{u.role === 'admin' ? '管理员' : '观看用户'}</td>
              <td className={u.is_active ? 'text-green-500' : 'text-red-400'}>
                {u.is_active ? '正常' : '已禁用'}
              </td>
              <td className="text-ink-50">
                {u.role === 'admin' ? '全部管理权限' : '仅浏览/播放/外部播放器，无下载与文件操作'}
              </td>
              <td className="text-ink-50">
                <span className="inline-flex flex-wrap items-center gap-2">
                  <span>{u.last_login_at ? new Date(u.last_login_at).toLocaleString() : '从未登录'}</span>
                  {u.realtime_online && <span className="rounded border border-green-400/40 px-1.5 py-0.5 text-[11px] text-green-500">在线</span>}
                  {(u.realtime_device_count ?? 0) > 0 && <span className="text-xs text-ink-50">{u.realtime_device_count} 台</span>}
                </span>
              </td>
              <td className="space-x-2 py-2 text-right">
                {editingID === u.id ? (
                  <>
                    <button
                      className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                      onClick={() => onSaveEdit(u.id)}
                    >
                      保存
                    </button>
                    <button
                      className="rounded-lg border border-gray-300 px-2 py-1 text-xs text-ink-100 hover:bg-gray-100"
                      onClick={onCancelEdit}
                    >
                      <X size={12} />
                    </button>
                  </>
                ) : (
                  <button
                    className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                    onClick={() => onStartEdit(u)}
                  >
                    <Pencil size={12} />
                  </button>
                )}
                <button
                  className="rounded-lg border border-amber-400/40 px-2 py-1 text-xs text-amber-500 hover:bg-amber-400/10"
                  title="重置密码"
                  disabled={resettingPasswordID === u.id}
                  onClick={() => onResetPassword(u)}
                >
                  {resettingPasswordID === u.id ? <Loader2 size={12} className="animate-spin" /> : <KeyRound size={12} />}
                </button>
                <button
                  className={
                    'rounded-lg border px-2 py-1 text-xs disabled:cursor-not-allowed disabled:opacity-40 ' +
                    (u.is_active
                      ? 'border-orange-400/40 text-orange-500 hover:bg-orange-400/10'
                      : 'border-green-400/40 text-green-500 hover:bg-green-400/10')
                  }
                  disabled={u.is_protected && u.is_active}
                  title={u.is_active ? '禁用用户' : '解禁用户'}
                  onClick={() => onToggleStatus(u)}
                >
                  {u.is_active ? <UserX size={12} /> : <UserCheck size={12} />}
                </button>
                <button
                  className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10 disabled:cursor-not-allowed disabled:opacity-40"
                  disabled={u.is_protected}
                  title={u.is_protected ? '默认管理员禁止删除' : '删除用户'}
                  onClick={() => onDeleteUser(u)}
                >
                  <Trash2 size={12} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
