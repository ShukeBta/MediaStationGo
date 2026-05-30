import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Cloud, FileVideo, Folder, Loader2, QrCode, Save, Send } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  cloudAPI,
  storageAPI,
  type CloudEntry,
  type QRSession,
  type StorageType,
} from '../api/storage_config'

const CLOUD_TYPES: StorageType[] = ['cloud115', 'quark']
const isCloud = (t: StorageType) => CLOUD_TYPES.includes(t)
const TYPE_LABEL: Record<string, string> = {
  alist: 'ALIST',
  webdav: 'WEBDAV',
  s3: 'S3',
  cloud115: '115网盘',
  quark: '夸克网盘',
}

// StorageConfigPage manages the Alist / S3 / WebDAV adapters used by
// the import / playback / STRM subsystems. Mirrors the Vue UI's
// `admin/storage/*` tabs in a tabbed React surface.
export function StorageConfigPage() {
  const [active, setActive] = useState<StorageType>('alist')
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-400/10 text-blue-300">
          <Cloud size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">外部存储</h1>
          <p className="text-sm text-ink-50">
            配置 Alist / S3 / WebDAV / 网盘(115 / 夸克)后端,Cookie 加密存储 + 在线测试,网盘资源通过 302 直链播放
          </p>
        </div>
      </div>

      <div className="flex gap-2 border-b border-gray-200">
        {(['alist', 'webdav', 's3', 'cloud115', 'quark'] as StorageType[]).map((t) => (
          <button
            key={t}
            onClick={() => setActive(t)}
            className={
              'border-b-2 px-4 py-2 text-sm ' +
              (active === t
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-ink-50 hover:text-white')
            }
          >
            {TYPE_LABEL[t] ?? t}
          </button>
        ))}
      </div>

      <StorageForm key={active} type={active} />
    </div>
  )
}

const FIELD_DEFS: Record<StorageType, { key: string; label: string; secret?: boolean; placeholder?: string }[]> = {
  alist: [
    { key: 'server', label: 'Server URL', placeholder: 'https://alist.example.com' },
    { key: 'token', label: 'Token', secret: true },
  ],
  webdav: [
    { key: 'url', label: 'URL', placeholder: 'https://example.com/dav/' },
    { key: 'username', label: '用户名' },
    { key: 'password', label: '密码', secret: true },
  ],
  s3: [
    { key: 'endpoint', label: 'Endpoint', placeholder: 'https://s3.amazonaws.com' },
    { key: 'region', label: 'Region', placeholder: 'us-east-1' },
    { key: 'bucket', label: 'Bucket' },
    { key: 'access_key', label: 'Access Key', secret: true },
    { key: 'secret_key', label: 'Secret Key', secret: true },
    { key: 'force_path_style', label: 'force_path_style (true/false)' },
  ],
  cloud115: [
    { key: 'cookie', label: 'Cookie(UID / CID / SEID,或扫码登录自动填充)', secret: true, placeholder: 'UID=...; CID=...; SEID=...' },
    { key: 'force_proxy', label: '强制反代(true/false,默认 302 直链)' },
  ],
  quark: [
    { key: 'cookie', label: 'Cookie(从 pan.quark.cn 复制整段)', secret: true, placeholder: '__pus=...; __kp=...; kps=...' },
    { key: 'force_302', label: '强制 302 直链(true/false,默认反代)' },
  ],
}

function StorageForm({ type }: { type: StorageType }) {
  const fields = useMemo(() => FIELD_DEFS[type], [type])
  const [config, setConfig] = useState<Record<string, string>>({})
  const [enabled, setEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  const refresh = async () => {
    setLoading(true)
    try {
      const r = await storageAPI.get(type)
      const next: Record<string, string> = {}
      for (const f of fields) {
        const v = r.config?.[f.key]
        // List() redacts secrets to "********"; show empty so the user
        // doesn't accidentally save the placeholder.
        next[f.key] = v === '********' ? '' : v ?? ''
      }
      setConfig(next)
      setEnabled(r.enabled)
    } catch {
      const next: Record<string, string> = {}
      for (const f of fields) next[f.key] = ''
      setConfig(next)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [type])

  const onSave = async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await storageAPI.save(type, config, enabled)
      toast.success('已保存')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const onTest = async () => {
    setTesting(true)
    try {
      const r = await storageAPI.test(type, config)
      if (r.ok) toast.success('连接成功')
      else toast.error(r.error ?? '连接失败')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '测试失败'
      toast.error(msg)
    } finally {
      setTesting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  return (
    <form onSubmit={onSave} className="glass-panel space-y-4">
      {fields.map((f) => (
        <label key={f.key} className="block">
          <span className="mb-1 block text-sm text-ink-100">
            {f.label}
            <span className="ml-2 font-mono text-[10px] text-gray-500">{f.key}</span>
          </span>
          <input
            type={f.secret ? 'password' : 'text'}
            className="input-base"
            placeholder={f.placeholder}
            value={config[f.key] ?? ''}
            onChange={(e) => setConfig((c) => ({ ...c, [f.key]: e.target.value }))}
          />
        </label>
      ))}
      {type === 'cloud115' && (
        <QRLoginPanel
          type={type}
          onCookie={(c) => setConfig((cfg) => ({ ...cfg, cookie: c }))}
        />
      )}
      <label className="flex items-center gap-2 text-sm text-ink-100">
        <input
          type="checkbox"
          className="h-4 w-4 accent-primary-400"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        启用
      </label>
      <div className="flex justify-end gap-2 pt-2">
        <button
          type="button"
          onClick={onTest}
          disabled={testing}
          className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50"
        >
          {testing ? <Loader2 size={14} className="inline animate-spin" /> : <Send size={14} className="inline" />}
          {' '}测试
        </button>
        <button type="submit" disabled={saving} className="neon-button">
          {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          保存
        </button>
      </div>
      {isCloud(type) && <CloudBrowser type={type} />}
    </form>
  )
}

// QRLoginPanel drives the 115 QR-code login: start → render image → poll →
// fill the cookie field on confirmation.
function QRLoginPanel({ type, onCookie }: { type: StorageType; onCookie: (c: string) => void }) {
  const [sess, setSess] = useState<QRSession | null>(null)
  const [state, setState] = useState<string>('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!sess) return
    let alive = true
    const timer = setInterval(async () => {
      try {
        const st = await cloudAPI.qrPoll(type, sess)
        if (!alive) return
        setState(st.state)
        if (st.state === 'confirmed' && st.cookie) {
          onCookie(st.cookie)
          toast.success('扫码登录成功,Cookie 已填入,请点击保存')
          setSess(null)
        } else if (st.state === 'expired') {
          toast.error('二维码已过期,请重新获取')
          setSess(null)
        }
      } catch {
        /* keep polling */
      }
    }, 2000)
    return () => {
      alive = false
      clearInterval(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sess])

  const start = async () => {
    setBusy(true)
    try {
      const s = await cloudAPI.qrStart(type)
      setSess(s)
      setState('waiting')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '获取二维码失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-lg border border-gray-200 p-3">
      <button
        type="button"
        onClick={start}
        disabled={busy}
        className="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:bg-gray-50"
      >
        {busy ? <Loader2 size={14} className="animate-spin" /> : <QrCode size={14} />}
        使用 115 App 扫码登录
      </button>
      {sess && (
        <div className="mt-3 flex items-center gap-3">
          <img src={sess.qr_image_url} alt="115 QR" className="h-40 w-40 rounded bg-white p-1" />
          <span className="text-sm text-ink-50">
            {state === 'scanned' ? '已扫描,请在手机上确认登录…' : '请使用 115 手机 App 扫描二维码…'}
          </span>
        </div>
      )}
    </div>
  )
}

// CloudBrowser lists 网盘 directories and imports a file as a 302-backed media.
function CloudBrowser({ type }: { type: StorageType }) {
  const [stack, setStack] = useState<{ id: string; name: string }[]>([{ id: '', name: '根目录' }])
  const [items, setItems] = useState<CloudEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const cur = stack[stack.length - 1]
  const load = async (dir: string) => {
    setLoading(true)
    setError('')
    try {
      const r = await cloudAPI.list(type, dir)
      setItems(r.items ?? [])
      if (r.error) setError(r.error)
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '加载失败')
      setItems([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(cur.id).catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stack.length, type])

  const enter = (e: CloudEntry) => setStack((s) => [...s, { id: e.id, name: e.name }])
  const goTo = (i: number) => setStack((s) => s.slice(0, i + 1))

  const doImport = async (e: CloudEntry) => {
    const ref = type === 'cloud115' ? e.pick_code || e.id : e.id
    try {
      await cloudAPI.import(type, ref, e.name, e.size)
      toast.success(`已导入「${e.name}」,可在媒体库中 302 播放`)
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '导入失败')
    }
  }

  return (
    <div className="mt-2 rounded-lg border border-gray-200 p-3" onClick={(e) => e.preventDefault()}>
      <div className="mb-2 flex flex-wrap items-center gap-1 text-xs text-ink-50">
        <span className="text-ink-100">网盘资源:</span>
        {stack.map((s, i) => (
          <span key={i}>
            <button type="button" className="hover:text-brand-500" onClick={() => goTo(i)}>
              {s.name}
            </button>
            {i < stack.length - 1 && <span className="mx-1">/</span>}
          </span>
        ))}
      </div>
      {loading ? (
        <div className="flex justify-center py-4 text-ink-50">
          <Loader2 className="animate-spin" size={16} />
        </div>
      ) : error ? (
        <p className="py-2 text-sm text-red-400">{error}(请先填写有效 Cookie 并保存)</p>
      ) : items.length === 0 ? (
        <p className="py-2 text-sm text-ink-50">该目录为空</p>
      ) : (
        <ul className="divide-y divide-gray-100">
          {items.map((e) => (
            <li key={e.id} className="flex items-center gap-2 py-1.5 text-sm">
              {e.is_dir ? <Folder size={15} className="text-amber-400" /> : <FileVideo size={15} className="text-blue-300" />}
              {e.is_dir ? (
                <button type="button" className="flex-1 text-left text-ink-100 hover:text-brand-500" onClick={() => enter(e)}>
                  {e.name}
                </button>
              ) : (
                <>
                  <span className="flex-1 truncate text-ink-100">{e.name}</span>
                  <button
                    type="button"
                    className="rounded border border-gray-200 px-2 py-0.5 text-xs text-ink-100 hover:bg-gray-50"
                    onClick={() => doImport(e)}
                  >
                    导入
                  </button>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
