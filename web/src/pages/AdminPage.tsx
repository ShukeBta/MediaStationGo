import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import { APIConfigsPanel } from '../components/APIConfigsPanel'
import { ManagementShortcuts } from '../components/ManagementShortcuts'
import { AdminLibraryPanel } from './AdminLibraryPanel'
import { AdminUsersPanel } from './AdminUsersPanel'

type AdminTab = 'library' | 'users' | 'api'

function parseAdminTab(value: string | null): AdminTab {
  if (value === 'users' || value === 'api') return value
  return 'library'
}

export function AdminPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [tab, setTab] = useState<AdminTab>(() => parseAdminTab(searchParams.get('tab')))
  const tabs = [
    { key: 'library' as const, label: '媒体库' },
    { key: 'users' as const, label: '用户' },
    { key: 'api' as const, label: '外部API' },
  ]

  useEffect(() => {
    setTab(parseAdminTab(searchParams.get('tab')))
  }, [searchParams])

  const selectTab = (next: AdminTab) => {
    setTab(next)
    setSearchParams(next === 'library' ? {} : { tab: next })
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">管理后台</h1>
      <ManagementShortcuts
        title="统一管理入口"
        description="侧栏保持精简，完整管理能力统一从这里进入。"
        items={[
          { to: '/sites', title: '站点管理', description: '维护 PT 站点、认证方式和检索配置', group: '站点与下载' },
          { to: '/download-clients', title: '下载器管理', description: '配置 qBittorrent 等下载器连接', badge: '下载', group: '站点与下载' },
          { to: '/files', title: '手动整理', description: '从下载目录选择文件夹并整理入库', group: '文件与入库' },
          { to: '/storage', title: '存储与文件', description: '查看占用、清理重复项和管理文件', group: '文件与入库' },
          { to: '/notify-channels', title: '通知渠道', description: '配置 Bot、Webhook、邮件等通知出口', group: '系统运维' },
          { to: '/assistant', title: 'AI 会话', description: '查看和接管后台 AI 操作记录', group: '系统运维' },
        ]}
      />
      <div className="flex flex-wrap gap-2 border-b border-gray-200">
        {tabs.map((k) => (
          <button
            key={k.key}
            onClick={() => selectTab(k.key)}
            className={
              'border-b-2 px-4 py-2 text-sm transition ' +
              (tab === k.key
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-ink-50 hover:text-white')
            }
          >
            {k.label}
          </button>
        ))}
      </div>

      {tab === 'library' && <AdminLibraryPanel />}
      {tab === 'users' && <AdminUsersPanel />}
      {tab === 'api' && <APIConfigsPanel />}
    </div>
  )
}
