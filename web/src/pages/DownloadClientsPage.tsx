import { useEffect, useState } from 'react'
import { Loader2, Plus, Server } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  downloadClientsAPI,
  type DownloadClient,
} from '../api/download_clients'
import { confirmAction } from '../components/confirmAction'
import { DownloadClientCard } from './DownloadClientCard'
import { ClientFormModal } from './DownloadClientFormModal'
import { apiErrorMessage } from './downloadClientPageModel'

// DownloadClientsPage manages multiple downloader integrations.
// Replaces the Vue UI's DownloadView "clients" tab with a typed CRUD
// surface and a per-client Test button.
export function DownloadClientsPage() {
  const [clients, setClients] = useState<DownloadClient[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<DownloadClient | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [testing, setTesting] = useState<Record<string, boolean>>({})

  const refresh = async () => {
    setLoading(true)
    try {
      setClients(await downloadClientsAPI.list())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onTest = async (id: string) => {
    if (testing[id]) return
    setTesting((current) => ({ ...current, [id]: true }))
    try {
      const r = await downloadClientsAPI.test(id)
      if (r.ok) toast.success('连接成功')
      else toast.error(r.error ?? '连接失败')
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '测试失败')
      toast.error(msg)
    } finally {
      setTesting((current) => ({ ...current, [id]: false }))
    }
  }

  const onDelete = async (c: DownloadClient) => {
    if (!(await confirmAction({ title: '删除下载器', message: `确定删除「${c.name}」?`, confirmText: '删除' }))) return
    try {
      await downloadClientsAPI.remove(c.id)
      toast.success('已删除')
      await refresh()
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '删除失败')
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-cyan-400/10 text-cyan-300">
            <Server size={20} />
          </div>
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">下载器管理</h1>
            <p className="text-sm text-ink-50">
              qBittorrent / Aria2 / Transmission · 多客户端 + 连接测试
            </p>
          </div>
        </div>
        <button
          onClick={() => {
            setEditing(null)
            setShowForm(true)
          }}
          className="neon-button"
        >
          <Plus size={16} /> 添加下载器
        </button>
      </div>

      {loading && (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}

      {!loading && clients.length === 0 && (
        <div className="glass-panel py-12 text-center text-ink-50">暂无下载器</div>
      )}

      {!loading && clients.length > 0 && (
        <div className="space-y-3">
          {clients.map((c) => (
            <DownloadClientCard
              key={c.id}
              client={c}
              testing={Boolean(testing[c.id])}
              onDelete={onDelete}
              onEdit={(client) => {
                setEditing(client)
                setShowForm(true)
              }}
              onTest={onTest}
            />
          ))}
        </div>
      )}

      {showForm && (
        <ClientFormModal
          editing={editing}
          onClose={() => setShowForm(false)}
          onSaved={async () => {
            setShowForm(false)
            refresh().catch((err: unknown) => toast.error(apiErrorMessage(err, '刷新下载器列表失败')))
          }}
        />
      )}
    </div>
  )
}
