import { FormEvent, useEffect, useState } from 'react'
import { FolderOpen, Loader2, Save, SettingsIcon } from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'
import type { Library, Setting } from '../types'

// SettingsPage replaces the Vue SettingsView's General / Organize /
// Scrape / Adult tabs. The Go backend stores settings as a single
// key/value table; we group the most useful keys client-side and let
// the operator edit them with typed widgets (select / toggle / input).
//
// Any key not in the curated list still works via the AdminPage generic
// key/value editor — this page is the curated UX over the same store.
interface SettingDef {
  key: string
  label: string
  type: 'text' | 'select' | 'toggle' | 'number' | 'textarea' | 'library-multiselect'
  hint?: string
  defaultValue?: string
  options?: { value: string; label: string }[]
  placeholder?: string
}

interface SettingGroup {
  key: string
  label: string
  description?: string
  items: SettingDef[]
}

const GROUPS: SettingGroup[] = [
  {
    key: 'general',
    label: '常规',
    description: '语言 / 转码引擎参数（API 密钥请在管理后台 → 外部API 配置）',
    items: [
      {
        key: 'tmdb.language',
        label: 'TMDb 元数据语言',
        type: 'select',
        options: [
          { value: 'zh-CN', label: '简体中文' },
          { value: 'zh-TW', label: '繁体中文' },
          { value: 'en-US', label: 'English' },
          { value: 'ja-JP', label: '日本語' },
        ],
      },
      {
        key: 'transcode.enabled',
        label: '启用转码',
        type: 'toggle',
        hint: '关闭后所有视频直连播放',
        defaultValue: 'true',
      },
      {
        key: 'transcode.hw_accel',
        label: '硬件编码器',
        type: 'select',
        hint: '只有开启下方「启用硬件加速」后才会使用；未开启时强制软件转码',
        defaultValue: 'none',
        options: [
          { value: 'none', label: '软件转码' },
          { value: 'nvenc', label: 'NVIDIA NVENC' },
          { value: 'qsv', label: 'Intel QSV' },
          { value: 'vaapi', label: 'VAAPI (Linux)' },
        ],
      },
      {
        key: 'transcode.hw_enabled',
        label: '启用硬件加速',
        type: 'toggle',
        hint: '关闭时即使选择了 NVENC/QSV/VAAPI，也不会调用硬件编码参数',
        defaultValue: 'false',
      },
      {
        key: 'transcode.max_jobs',
        label: '最大并发转码任务',
        type: 'number',
        hint: 'NAS 建议 1',
        defaultValue: '1',
      },
      {
        key: 'transcode.realtime',
        label: '按播放速度转码',
        type: 'toggle',
        hint: '开启后 ffmpeg 不会抢跑压完整片，可显著降低 CPU 峰值',
        defaultValue: 'true',
      },
      {
        key: 'transcode.threads',
        label: '软件转码线程数',
        type: 'number',
        hint: 'NAS 建议 1-2；仅软件转码生效',
        defaultValue: '2',
      },
      {
        key: 'transcode.idle_timeout_seconds',
        label: '转码空闲停止秒数',
        type: 'number',
        hint: '播放器关闭或停止请求分片后自动结束 ffmpeg',
        defaultValue: '120',
      },
      {
        key: 'ffmpeg.path',
        label: 'FFmpeg 路径',
        type: 'text',
        placeholder: 'ffmpeg',
      },
      {
        key: 'ffprobe.path',
        label: 'FFprobe 路径',
        type: 'text',
        placeholder: 'ffprobe',
      },
    ],
  },
  {
    key: 'license',
    label: '授权服务',
    description: '连接私有 MediaStationLicenseServer；开源版默认最多 20 个用户，激活后按授权策略提升额度。',
    items: [
      {
        key: 'license.server_url',
        label: 'License Server 地址',
        type: 'text',
        placeholder: 'http://127.0.0.1:8001',
      },
      {
        key: 'license.hmac_secret',
        label: 'HMAC 签名密钥',
        type: 'text',
        hint: '必须与 License Server 的 LICENSE_HMAC_SECRET 保持一致；留空则跳过响应签名校验。',
      },
    ],
  },
  {
    key: 'organize',
    label: '整理 & 刮削',
    description: '媒体文件命名 + 自动刮削 + 整理目标',
    items: [
      {
        key: 'organizer.auto_after_download',
        label: '入库时自动整理',
        type: 'toggle',
      },
      {
        key: 'organizer.smart_classify',
        label: '启用智能分类',
        type: 'toggle',
        hint: '根据元数据（语言/国家/类型）自动分类到子目录（如：华语电影、欧美剧、日番）',
      },
      {
        key: 'organize.source_dir',
        label: '整理源目录（待整理）',
        type: 'text',
        hint: '从该目录读取待整理文件；留空则默认整理整个媒体库（媒体库路径）。',
        placeholder: '/mnt/downloads',
      },
      {
        key: 'organize.target_dir',
        label: '整理目的地目录',
        type: 'text',
        hint: '整理后输出到该目录；留空则默认整理到各媒体库对应路径（见下方参考）。与「源目录」相互独立。',
        placeholder: '/mnt/media/organized',
      },
      {
        key: 'organize.transfer_mode',
        label: '默认转移方式',
        type: 'select',
        hint: '移动会删除源文件；复制/硬链接/软链接保留源文件，PT 做种不中断。硬链接同盘零额外占用。',
        options: [
          { value: 'move', label: '移动（删除源文件）' },
          { value: 'copy', label: '复制（保留源文件）' },
          { value: 'hardlink', label: '硬链接（保留源，做种不中断，不占双倍空间）' },
          { value: 'symlink', label: '软链接（符号链接，保留源）' },
        ],
      },
      {
        key: 'organize.keep_seeding',
        label: '保种（整理后继续做种上传）',
        type: 'toggle',
        hint: '开启后即使选择「移动」也会自动改用硬链接（跨盘退化为复制）保留源文件，确保 qBittorrent 转移后继续做种上传。',
      },
      {
        key: 'organize.movie_format',
        label: '电影命名格式',
        type: 'text',
        hint: '例: {title} ({year})/{title} ({year})',
        placeholder: '{title} ({year})/{title} ({year})',
      },
      {
        key: 'organize.tv_format',
        label: '剧集命名格式',
        type: 'text',
        placeholder: '{title} ({year})/Season {season}/{title} S{season:02}E{episode:02}',
      },
      {
        key: 'organize.anime_format',
        label: '动漫命名格式',
        type: 'text',
        placeholder: '{title}/Season {season}/{title} S{season:02}E{episode:02}',
      },
      {
        key: 'scrape.auto_on_scan',
        label: '扫描后自动刮削',
        type: 'toggle',
      },
      {
        key: 'scrape.providers',
        label: '刮削源优先级',
        type: 'text',
        hint: '逗号分隔: tmdb,bangumi,thetvdb,fanart',
        placeholder: 'tmdb,bangumi,thetvdb,fanart',
      },
      {
        key: 'scrape.language',
        label: '刮削首选语言',
        type: 'text',
        placeholder: 'zh-CN',
      },
      {
        key: 'scan.periodic_enabled',
        label: '周期性整库重扫',
        type: 'toggle',
        hint: '默认关闭。文件新增/变更由实时监听增量入库，无需定时全量重扫。开启会每 60 分钟重扫整库，频繁读盘会损伤硬盘，一般无需开启。',
      },
    ],
  },
  {
    key: 'adult',
    label: 'Adult / NSFW',
    description: '成人内容隔离开关 (默认隐藏)',
    items: [
      {
        key: 'adult.enabled',
        label: '启用成人内容',
        type: 'toggle',
        hint: '全局开关。关闭后所有人都无法显示成人库；开启后用户仍默认隐藏，可在个人资料或 Bot 中自行显示。',
        defaultValue: 'true',
      },
      {
        key: 'adult.library_ids',
        label: '指定成人媒体库',
        type: 'library-multiselect',
        hint: '管理员指定哪些媒体库目录属于成人影视库。指定后网页、搜索和第三方客户端都会统一隐藏。',
        defaultValue: '[]',
      },
      {
        key: 'adult.require_pin',
        label: '访问需要 PIN',
        type: 'toggle',
      },
      {
        key: 'adult.pin',
        label: 'PIN 码',
        type: 'text',
        hint: '4-8 位数字',
      },
    ],
  },
  // qBittorrent 配置已迁移到独立的「下载器」页面（侧边栏 → 下载器），
  // 该页面支持多客户端 + 连接测试。这里不再重复暴露入口，避免与
  // /api/admin/download/clients 写入的数据来源冲突。
]

const ALL_KEYS = new Set(GROUPS.flatMap((g) => g.items.map((i) => i.key)))

export function SettingsPage() {
  const [activeGroup, setActiveGroup] = useState(GROUPS[0].key)
  const [values, setValues] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])

  const refresh = async () => {
    setLoading(true)
    try {
      const [all, libs] = await Promise.all([
        adminAPI.listSettings(),
        libraryAPI.list({ includeHidden: true }).catch(() => [] as Library[]),
      ])
      const idx: Record<string, string> = {}
      for (const s of all as Setting[]) {
        if (ALL_KEYS.has(s.key)) idx[s.key] = s.value
      }
      setValues(idx)
      setLibraries(libs as Library[])
      setDirty(new Set())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onChange = (key: string, value: string) => {
    setValues((v) => ({ ...v, [key]: value }))
    setDirty((d) => new Set(d).add(key))
  }

  const onSave = async (e: FormEvent) => {
    e.preventDefault()
    if (dirty.size === 0) return
    setSaving(true)
    try {
      // Backend exposes a single-key updater; loop through dirty keys.
      for (const key of dirty) {
        await adminAPI.updateSetting(key, values[key] ?? '')
      }
      toast.success(`已保存 ${dirty.size} 项配置`)
      setDirty(new Set())
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const group = GROUPS.find((g) => g.key === activeGroup)!

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sand-300/40 text-ink-100">
          <SettingsIcon size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">系统设置</h1>
          <p className="text-sm text-ink-50">
            按分组编辑转码 / 整理 / 刮削 / 下载器等关键配置
          </p>
        </div>
      </div>

      <div className="flex gap-2 overflow-x-auto border-b border-gray-200">
        {GROUPS.map((g) => (
          <button
            key={g.key}
            onClick={() => setActiveGroup(g.key)}
            className={
              'border-b-2 px-4 py-2 text-sm whitespace-nowrap transition ' +
              (activeGroup === g.key
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-ink-50 hover:text-white')
            }
          >
            {g.label}
          </button>
        ))}
      </div>

      {loading && (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}

      {!loading && (
        <form onSubmit={onSave} className="glass-panel space-y-4">
          {group.description && <p className="text-xs text-sand-500">{group.description}</p>}
          {group.items.map((it) => (
            <SettingRow
              key={it.key}
              def={it}
              value={values[it.key] ?? it.defaultValue ?? ''}
              onChange={(v) => onChange(it.key, v)}
              libraries={libraries}
            />
          ))}
          <div className="flex items-center justify-between pt-2">
            <span className="text-xs text-sand-500">
              {dirty.size > 0 ? `有 ${dirty.size} 项未保存` : '所有更改已保存'}
            </span>
            <button
              type="submit"
              disabled={saving || dirty.size === 0}
              className="neon-button disabled:opacity-50"
            >
              {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
              保存
            </button>
          </div>
        </form>
      )}

      {/* 整理 tab 时显示各媒体库默认路径 */}
      {!loading && activeGroup === 'organize' && libraries.length > 0 && (
        <div className="glass-panel">
          <div className="mb-3 flex items-center gap-2 text-sm text-ink-100">
            <FolderOpen size={16} className="text-brand-500" />
            <span>默认整理路径参考（未设目的地目录时按媒体库归类）</span>
          </div>
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="py-2">媒体库</th>
                <th>类型</th>
                <th>路径</th>
                <th>整理后示例</th>
              </tr>
            </thead>
            <tbody>
              {libraries.map((lib) => (
                <tr key={lib.id} className="border-t border-gray-200">
                  <td className="py-2 font-medium text-ink-600">{lib.name}</td>
                  <td className="text-ink-50">
                    {lib.type === 'movie' ? '电影' : lib.type === 'tv' ? '电视剧' : lib.type === 'anime' ? '动漫' : '音乐'}
                  </td>
                  <td className="font-mono text-xs text-ink-50">{lib.path}</td>
                  <td className="font-mono text-[11px] text-sand-500">
                    {lib.type === 'movie'
                      ? `${lib.path}/片名 (2024)/片名 (2024).mkv`
                      : lib.type === 'tv' || lib.type === 'anime'
                        ? `${lib.path}/剧名/Season 01/剧名 - S01E01.mkv`
                        : lib.path}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function SettingRow({
  def,
  value,
  onChange,
  libraries = [],
}: {
  def: SettingDef
  value: string
  onChange: (v: string) => void
  libraries?: Library[]
}) {
  const toggleOn = value === 'true' || value === '1' || value === 'on'
  const selectedLibraryIDs = parseLibraryIDs(value)
  const toggleLibrary = (id: string, checked: boolean) => {
    const next = checked
      ? Array.from(new Set([...selectedLibraryIDs, id]))
      : selectedLibraryIDs.filter((item) => item !== id)
    onChange(JSON.stringify(next))
  }
  return (
    <div className="grid items-start gap-2 md:grid-cols-[280px_1fr]">
      <label className="text-sm text-ink-100">
        <div className="font-medium">{def.label}</div>
        {def.hint && <div className="mt-0.5 text-xs text-sand-500">{def.hint}</div>}
        <div className="mt-0.5 font-mono text-[10px] text-gray-500">{def.key}</div>
      </label>
      <div>
        {def.type === 'text' && (
          <input
            className="input-base"
            value={value}
            placeholder={def.placeholder}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
        {def.type === 'number' && (
          <input
            type="number"
            className="input-base"
            value={value}
            placeholder={def.placeholder}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
        {def.type === 'textarea' && (
          <textarea
            rows={3}
            className="input-base font-mono text-xs"
            value={value}
            placeholder={def.placeholder}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
        {def.type === 'select' && (
          <select className="input-base" value={value} onChange={(e) => onChange(e.target.value)}>
            <option value="">(未设置)</option>
            {def.options?.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        )}
        {def.type === 'toggle' && (
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              className="h-4 w-4 accent-primary-400"
              checked={toggleOn}
              onChange={(e) => onChange(e.target.checked ? 'true' : 'false')}
            />
            <span className="text-sm text-ink-100">{toggleOn ? '已启用' : '已关闭'}</span>
          </label>
        )}
        {def.type === 'library-multiselect' && (
          <div className="space-y-2 rounded-2xl border border-gray-200 bg-white/70 p-3">
            {libraries.length === 0 && (
              <div className="text-sm text-ink-50">暂无媒体库，请先到「媒体与用户 → 媒体库」添加。</div>
            )}
            {libraries.map((lib) => (
              <label key={lib.id} className="flex cursor-pointer items-start gap-3 rounded-xl px-2 py-2 hover:bg-sand-100/50">
                <input
                  type="checkbox"
                  className="mt-1 h-4 w-4 accent-primary-400"
                  checked={selectedLibraryIDs.includes(lib.id)}
                  onChange={(e) => toggleLibrary(lib.id, e.target.checked)}
                />
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-ink-600">{lib.name}</span>
                  <span className="block truncate font-mono text-xs text-ink-50">{lib.path}</span>
                </span>
              </label>
            ))}
            <div className="text-xs text-sand-500">
              已选择 {selectedLibraryIDs.length} 个成人媒体库；新用户默认隐藏，用户可在个人资料或 Bot 中显示/隐藏。
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function parseLibraryIDs(raw: string): string[] {
  try {
    const parsed = JSON.parse(raw || '[]')
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item).trim()).filter(Boolean)
    }
  } catch {
    return raw
      .split(/[,\n;，]/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}
