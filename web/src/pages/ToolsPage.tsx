import { FormEvent, useEffect, useState } from 'react'
import { Bell, FolderCog, Info, Loader2, Search, Wrench } from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI, mediaAPI } from '../api/library'
import { toolsAPI } from '../api/tools'
import type { Library, Media, Setting } from '../types'
import { ManagementShortcuts } from '../components/ManagementShortcuts'

// ToolsPage gathers admin-only one-off operations that don't belong on a
// dedicated screen of their own:
//
//   - Organize one media item or an entire library: renames + moves files
//     into the canonical layout (`Library/Year/Title (Year)/...`).
//   - Send a test notification through every configured channel.
//
// The original Vue version surfaces these as tabs inside SettingsView; here
// we co-locate them on a single admin tools page.
export function ToolsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-400/10 text-violet-400">
          <Wrench size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">运维工具</h1>
          <p className="text-sm text-ink-50">整理媒体文件 · 测试通知渠道</p>
        </div>
      </div>

      <ManagementShortcuts
        title="运维与自动化入口"
        description="把整理、任务、通知和高级辅助功能统一放回工具台。"
        items={[
          { to: '/strm', title: 'STRM 生成', description: '生成 STRM 文件供外部播放器或媒体服务使用' },
          { to: '/scheduler', title: '定时任务', description: '查看和维护自动扫描、刮削与订阅任务' },
          { to: '/tasks', title: '任务队列', description: '跟踪后台任务执行状态和失败原因' },
          { to: '/notify-channels', title: '通知渠道', description: '配置并测试消息通知渠道' },
          { to: '/assistant', title: 'AI 对话台', description: '进入管理员辅助诊断和问答界面' },
        ]}
      />

      <OrganizePanel />
      <NotifyPanel />
    </div>
  )
}

function OrganizePanel() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [libraryID, setLibraryID] = useState('')
  const [running, setRunning] = useState(false)

  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState<Media[]>([])
  const [busyID, setBusyID] = useState<string | null>(null)

  const [smartClassify, setSmartClassify] = useState(false)
  const [loadingSettings, setLoadingSettings] = useState(true)

  // 单次整理覆盖项：留空则沿用设置页的默认整理目录与转移方式。
  const [targetPath, setTargetPath] = useState('')
  const [transferMode, setTransferMode] = useState('')

  const overrides = () => {
    const o: { target_path?: string; transfer_mode?: string } = {}
    if (targetPath.trim()) o.target_path = targetPath.trim()
    if (transferMode) o.transfer_mode = transferMode
    return o
  }

  useEffect(() => {
    libraryAPI.list().then(setLibraries).catch(() => undefined)
  }, [])

  useEffect(() => {
    const loadSettings = async () => {
      try {
        const settings = await adminAPI.listSettings()
        const setting = (settings as Setting[]).find(s => s.key === 'organizer.smart_classify')
        if (setting) {
          setSmartClassify(setting.value === 'true' || setting.value === '1' || setting.value === 'on')
        }
      } catch {
        // ignore
      } finally {
        setLoadingSettings(false)
      }
    }
    loadSettings()
  }, [])

  const onOrganizeLibrary = async (e: FormEvent) => {
    e.preventDefault()
    if (!libraryID) return
    setRunning(true)
    try {
      await toolsAPI.organizeLibrary(libraryID, overrides())
      toast.success('已触发媒体库整理')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '整理失败'
      toast.error(msg)
    } finally {
      setRunning(false)
    }
  }

  const doSearch = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!query.trim()) return
    setSearching(true)
    try {
      const r = await mediaAPI.search(query.trim(), 20)
      setResults(r.items ?? [])
    } catch {
      toast.error('搜索失败')
    } finally {
      setSearching(false)
    }
  }

  const onOrganizeOne = async (m: Media) => {
    setBusyID(m.id)
    try {
      const r = await toolsAPI.organizeMedia(m.id, overrides())
      toast.success(`已整理到 ${r.path}`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '整理失败'
      toast.error(msg)
    } finally {
      setBusyID(null)
    }
  }

  return (
    <section className="glass-panel space-y-5">
      <div className="flex items-center gap-2">
        <FolderCog size={18} className="text-brand-500" />
        <h2 className="font-display text-lg font-semibold text-ink-600">整理 &amp; 重命名</h2>
      </div>
      <p className="text-xs text-sand-500">
        将媒体文件按 <code className="rounded-lg bg-gray-50 px-1">媒体库/电影或电视剧/分类/标题/Season</code>{' '}
        的规范布局移动并重命名,确保已先完成刮削。
      </p>

      {!loadingSettings && (
        <div className="flex items-center gap-2 text-xs">
          <Info size={14} className={smartClassify ? 'text-green-400' : 'text-sand-500'} />
          <span className={smartClassify ? 'text-green-400' : 'text-sand-500'}>
            智能分类：{smartClassify ? '已启用' : '未启用'}
          </span>
          {smartClassify && (
            <span className="text-sand-500">（将根据元数据自动分类到子目录）</span>
          )}
        </div>
      )}

      <div className="grid gap-3 rounded-xl border border-gray-200 bg-gray-50 p-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-xs text-ink-50">整理目标目录（可选，覆盖默认设置）</span>
          <input
            className="input-base w-full"
            placeholder="留空则使用「整理目标目录」设置或媒体库路径"
            value={targetPath}
            onChange={(e) => setTargetPath(e.target.value)}
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">转移方式（可选，覆盖默认设置）</span>
          <select
            className="input-base w-full"
            value={transferMode}
            onChange={(e) => setTransferMode(e.target.value)}
          >
            <option value="">使用默认设置</option>
            <option value="move">移动（删除源文件）</option>
            <option value="copy">复制（保留源文件）</option>
            <option value="hardlink">硬链接（保留源，做种不中断）</option>
            <option value="symlink">软链接（符号链接，保留源）</option>
          </select>
        </label>
      </div>

      <form onSubmit={onOrganizeLibrary} className="flex flex-wrap gap-2">
        <select
          required
          className="input-base flex-1 min-w-[200px]"
          value={libraryID}
          onChange={(e) => setLibraryID(e.target.value)}
        >
          <option value="" disabled>
            选择要整理的媒体库
          </option>
          {libraries.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name} ({l.type})
            </option>
          ))}
        </select>
        <button type="submit" disabled={running || !libraryID} className="neon-button">
          {running ? <Loader2 size={16} className="animate-spin" /> : <FolderCog size={16} />}
          整理整个库
        </button>
      </form>

      <div className="border-t border-gray-200 pt-4">
        <form onSubmit={doSearch} className="flex flex-wrap gap-2">
          <input
            className="input-base flex-1"
            placeholder="或搜索单个媒体进行整理…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button type="submit" disabled={searching} className="neon-button">
            <Search size={16} /> {searching ? '搜索中…' : '搜索'}
          </button>
        </form>

        {results.length > 0 && (
          <ul className="mt-4 space-y-2">
            {results.map((m) => (
              <li
                key={m.id}
                className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2"
              >
                <div className="min-w-0">
                  <div className="truncate text-sm text-ink-600">{m.title}</div>
                  <div className="truncate text-xs text-ink-50">{m.path}</div>
                </div>
                <button
                  onClick={() => onOrganizeOne(m)}
                  disabled={busyID === m.id}
                  className="neon-button !px-3 !py-1 text-xs"
                >
                  {busyID === m.id ? (
                    <Loader2 size={12} className="animate-spin" />
                  ) : (
                    <FolderCog size={12} />
                  )}
                  整理
                </button>
              </li>
            ))}
          </ul>
        )}

        {!searching && query && results.length === 0 && (
          <p className="mt-3 text-sm text-ink-50">未找到匹配的媒体。</p>
        )}
      </div>
    </section>
  )
}

function NotifyPanel() {
  const [title, setTitle] = useState('MediaStation 测试通知')
  const [body, setBody] = useState('如果你收到这条消息,说明通知渠道工作正常。')
  const [sending, setSending] = useState(false)

  const onSend = async (e: FormEvent) => {
    e.preventDefault()
    if (!title.trim() || !body.trim()) return
    setSending(true)
    try {
      await toolsAPI.notifyTest(title.trim(), body.trim())
      toast.success('已派发到所有已启用通道')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '发送失败'
      toast.error(msg)
    } finally {
      setSending(false)
    }
  }

  return (
    <section className="glass-panel space-y-4">
      <div className="flex items-center gap-2">
        <Bell size={18} className="text-amber-300" />
        <h2 className="font-display text-lg font-semibold text-ink-600">通知渠道测试</h2>
      </div>
      <p className="text-xs text-sand-500">
        会向所有已配置的通知渠道(Telegram / Bark / Webhook 等)发送一条测试消息。
      </p>
      <form onSubmit={onSend} className="space-y-3">
        <Field label="标题">
          <input
            required
            className="input-base"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
        </Field>
        <Field label="内容">
          <textarea
            required
            rows={3}
            className="input-base resize-none"
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
        </Field>
        <button type="submit" disabled={sending} className="neon-button">
          {sending ? <Loader2 size={16} className="animate-spin" /> : <Bell size={16} />}
          发送测试通知
        </button>
      </form>
    </section>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-ink-100">{label}</span>
      {children}
    </label>
  )
}
