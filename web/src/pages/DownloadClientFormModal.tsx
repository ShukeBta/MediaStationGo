import { type FormEvent, type ReactNode, useState } from 'react'
import { Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  downloadClientsAPI,
  type DownloadClient,
  type DownloadClientInput,
  type DownloadClientType,
} from '../api/download_clients'
import { apiErrorMessage } from './downloadClientPageModel'

export function ClientFormModal({
  editing,
  onClose,
  onSaved,
}: {
  editing: DownloadClient | null
  onClose: () => void
  onSaved: () => void | Promise<void>
}) {
  const [form, setForm] = useState<DownloadClientInput>(() => ({
    name: editing?.name ?? '',
    type: editing?.type ?? 'qbittorrent',
    host: editing?.host ?? '',
    username: editing?.username ?? '',
    password: '',
    is_default: editing?.is_default ?? false,
    enabled: editing?.enabled ?? true,
  }))
  const [saving, setSaving] = useState(false)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (saving) return
    setSaving(true)
    try {
      if (editing) await downloadClientsAPI.update(editing.id, form)
      else await downloadClientsAPI.create(form)
      toast.success('已保存')
      setSaving(false)
      onSaved()
    } catch (err: unknown) {
      const msg = apiErrorMessage(err, '保存失败')
      toast.error(msg)
      setSaving(false)
    }
  }

  const update = <K extends keyof DownloadClientInput>(k: K, v: DownloadClientInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }))

  const placeholder = (
    {
      qbittorrent: 'http://127.0.0.1:8080',
      aria2: 'http://127.0.0.1:6800/jsonrpc',
      transmission: 'http://127.0.0.1:9091/transmission/rpc',
    } as Record<DownloadClientType, string>
  )[form.type]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="glass-panel w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="mb-4 font-display text-xl font-semibold text-ink-600">
          {editing ? '编辑下载器' : '添加下载器'}
        </h2>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label="名称">
            <input
              required
              className="input-base"
              value={form.name}
              onChange={(e) => update('name', e.target.value)}
            />
          </Field>
          <Field label="类型">
            <select
              className="input-base"
              value={form.type}
              onChange={(e) => update('type', e.target.value as DownloadClientType)}
            >
              <option value="qbittorrent">qBittorrent</option>
              <option value="aria2">Aria2 (JSON-RPC)</option>
              <option value="transmission">Transmission</option>
            </select>
          </Field>
          <Field label="URL">
            <input
              required
              className="input-base"
              placeholder={placeholder}
              value={form.host}
              onChange={(e) => update('host', e.target.value)}
            />
          </Field>
          {form.type !== 'aria2' && (
            <>
              <Field label="用户名">
                <input
                  className="input-base"
                  value={form.username ?? ''}
                  onChange={(e) => update('username', e.target.value)}
                />
              </Field>
              <Field label={editing ? '密码 (留空保持不变)' : '密码'}>
                <input
                  type="password"
                  className="input-base"
                  value={form.password ?? ''}
                  onChange={(e) => update('password', e.target.value)}
                />
              </Field>
            </>
          )}
          {form.type === 'aria2' && (
            <Field label="RPC Token (作为密码字段保存)">
              <input
                type="password"
                className="input-base"
                value={form.password ?? ''}
                onChange={(e) => update('password', e.target.value)}
              />
            </Field>
          )}
          <div className="flex flex-wrap gap-4">
            <label className="flex items-center gap-2 text-sm text-ink-100">
              <input
                type="checkbox"
                className="h-4 w-4 accent-primary-400"
                checked={form.is_default}
                onChange={(e) => update('is_default', e.target.checked)}
              />
              设为默认
            </label>
            <label className="flex items-center gap-2 text-sm text-ink-100">
              <input
                type="checkbox"
                className="h-4 w-4 accent-primary-400"
                checked={form.enabled}
                onChange={(e) => update('enabled', e.target.checked)}
              />
              启用
            </label>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50"
            >
              取消
            </button>
            <button type="submit" disabled={saving} className="neon-button">
              {saving && <Loader2 size={16} className="animate-spin" />} 保存
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-ink-100">{label}</span>
      {children}
    </label>
  )
}
