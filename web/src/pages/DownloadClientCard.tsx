import { Loader2, Pencil, Send, Trash2 } from 'lucide-react'

import type { DownloadClient } from '../api/download_clients'

export function DownloadClientCard({
  client,
  testing,
  onDelete,
  onEdit,
  onTest,
}: {
  client: DownloadClient
  testing: boolean
  onDelete: (client: DownloadClient) => void
  onEdit: (client: DownloadClient) => void
  onTest: (id: string) => void
}) {
  return (
    <div className="glass-panel flex items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-ink-600">{client.name}</span>
          <span className="rounded-lg border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-ink-50">
            {client.type}
          </span>
          {client.is_default && (
            <span className="rounded-lg bg-primary-400/20 px-2 py-0.5 text-xs text-brand-500">
              默认
            </span>
          )}
          {!client.enabled && (
            <span className="rounded-lg bg-sand-500/30 px-2 py-0.5 text-xs text-ink-100">
              已禁用
            </span>
          )}
        </div>
        <div className="mt-1 truncate text-xs text-ink-50">
          {client.host}
          {client.username && ` · ${client.username}`}
        </div>
      </div>
      <div className="flex shrink-0 gap-2">
        <button
          onClick={() => onTest(client.id)}
          disabled={testing}
          className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
        >
          {testing ? (
            <Loader2 size={12} className="inline animate-spin" />
          ) : (
            <Send size={12} className="inline" />
          )}{' '}
          测试
        </button>
        <button
          onClick={() => onEdit(client)}
          className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
        >
          <Pencil size={12} className="inline" /> 编辑
        </button>
        <button
          onClick={() => onDelete(client)}
          className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
        >
          <Trash2 size={12} className="inline" /> 删除
        </button>
      </div>
    </div>
  )
}
