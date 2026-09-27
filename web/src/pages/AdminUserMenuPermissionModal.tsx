import { useState } from 'react'
import { RotateCcw, Save, ShieldCheck, X } from 'lucide-react'

import type { User, UserPermission } from '../types'
import { AdminAccessSwitch } from './AdminAccessSwitch'

type MenuPermissionKey =
  | 'can_view_dashboard'
  | 'can_play_media'
  | 'can_favorite'
  | 'can_view_history'
  | 'can_view_discover'
  | 'can_use_ai'
  | 'can_cast'
  | 'can_use_ai_assistant'
  | 'can_manage_downloads'
  | 'can_manage_files'

const menuPermissionGroups: Array<{
  title: string
  items: Array<{ key: MenuPermissionKey; label: string; description: string }>
}> = [
  {
    title: '媒体浏览',
    items: [
      { key: 'can_view_dashboard', label: '系统首页', description: '首页推荐和继续观看入口' },
      { key: 'can_play_media', label: '媒体库与播放', description: '媒体库、海报墙、播放列表、详情与播放页' },
      { key: 'can_favorite', label: '我的收藏', description: '收藏菜单和收藏操作' },
      { key: 'can_view_history', label: '观看历史', description: '观看历史菜单和历史记录' },
    ],
  },
  {
    title: '发现与工具',
    items: [
      { key: 'can_view_discover', label: '精彩发现', description: '发现页及成人专区' },
      { key: 'can_use_ai', label: '智能搜索', description: 'AI 搜索与推荐入口' },
      { key: 'can_cast', label: 'DLNA 投屏', description: '设备发现和投屏控制' },
      { key: 'can_use_ai_assistant', label: 'AI 助理', description: 'AI 助理菜单' },
    ],
  },
  {
    title: '下载与维护',
    items: [
      { key: 'can_manage_downloads', label: '下载中心', description: '查看和管理下载任务' },
      { key: 'can_manage_files', label: '回收站', description: '查看、恢复和清理回收站' },
    ],
  },
]

function permissionDraft(permission: UserPermission): Record<string, boolean> {
  return Object.fromEntries(
    Object.entries(permission).filter(([key, value]) => key.startsWith('can_') && typeof value === 'boolean'),
  )
}

export function AdminUserMenuPermissionModal({
  user,
  permission,
  saving,
  resetting,
  onClose,
  onSave,
  onReset,
}: {
  user: User
  permission: UserPermission
  saving: boolean
  resetting: boolean
  onClose: () => void
  onSave: (permissions: Record<string, boolean>) => void
  onReset: () => void
}) {
  const [draft, setDraft] = useState<Record<string, boolean>>(() => permissionDraft(permission))
  const busy = saving || resetting

  const toggle = (key: MenuPermissionKey, checked: boolean) => {
    setDraft((current) => ({ ...current, [key]: checked }))
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4" role="dialog" aria-modal="true">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-hidden rounded-lg border border-gray-200 bg-white shadow-2xl">
        <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
          <div className="flex min-w-0 items-center gap-2">
            <ShieldCheck size={18} className="shrink-0 text-brand-500" />
            <div className="min-w-0">
              <h2 className="truncate text-base font-semibold text-ink-600">菜单权限</h2>
              <p className="truncate text-xs text-ink-50">{user.username}</p>
            </div>
          </div>
          <button type="button" className="icon-button" title="关闭" onClick={onClose} disabled={busy}>
            <X size={16} />
          </button>
        </div>

        <div className="max-h-[65vh] space-y-5 overflow-y-auto p-4">
          {menuPermissionGroups.map((group) => (
            <section key={group.title} className="space-y-2">
              <h3 className="text-xs font-bold uppercase tracking-wider text-sand-500">{group.title}</h3>
              <div className="divide-y divide-gray-100 rounded-lg border border-gray-200">
                {group.items.map((item) => (
                  <div key={item.key} className="flex items-center justify-between gap-4 px-3 py-3">
                    <span className="min-w-0">
                      <span className="block text-sm font-medium text-ink-600">{item.label}</span>
                      <span className="block text-xs text-ink-50">{item.description}</span>
                    </span>
                    <AdminAccessSwitch
                      checked={draft[item.key] === true}
                      disabled={busy}
                      label={`${draft[item.key] ? '关闭' : '开放'}${item.label}`}
                      onChange={(checked) => toggle(item.key, checked)}
                    />
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>

        <div className="flex flex-wrap justify-between gap-2 border-t border-gray-200 px-4 py-3">
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-lg border border-amber-300 px-3 py-2 text-sm text-amber-700"
            disabled={busy}
            onClick={onReset}
          >
            <RotateCcw size={15} />
            {resetting ? '重置中' : '恢复默认'}
          </button>
          <div className="flex gap-2">
            <button type="button" className="rounded-lg border border-gray-300 px-3 py-2 text-sm text-ink-100" onClick={onClose} disabled={busy}>
              取消
            </button>
            <button type="button" className="neon-button inline-flex items-center gap-2" onClick={() => onSave(draft)} disabled={busy}>
              <Save size={15} />
              {saving ? '保存中' : '保存'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
