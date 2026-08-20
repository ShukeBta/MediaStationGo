import { FormEvent, useEffect, useState } from 'react'
import {
  Activity,
  CheckCircle2,
  Cpu,
  Database,
  Eye,
  EyeOff,
  Globe,
  Loader2,
  Lock,
  Save,
  Server,
  Sparkles,
  XCircle,
} from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'
import type { Library, Setting } from '../types'
import { adultSettingsGroup } from './settingsGroupAccess'

const POPULAR_METATUBE_PROVIDERS = [
  { id: '', label: '自动 (全部)' },
  { id: 'javdb', label: 'JavDB' },
  { id: 'javbus', label: 'JavBus' },
  { id: 'dmm', label: 'DMM / FANZA' },
  { id: 'fc2', label: 'FC2' },
  { id: 'gfriends', label: 'GFriends' },
  { id: 'airav', label: 'AirAV' },
  { id: 'heyzo', label: 'HEYZO' },
  { id: 'mgs', label: 'MGS' },
]

export function AdultSettingsPanel() {
  const [values, setValues] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])
  const [showToken, setShowToken] = useState(false)

  // Test connection state
  const [testingMT, setTestingMT] = useState(false)
  const [mtTestResult, setMtTestResult] = useState<{
    success?: boolean
    latency_ms?: number
    providers?: string[]
    error?: string
  } | null>(null)

  const [testingBuiltin, setTestingBuiltin] = useState(false)
  const [builtinTestResult, setBuiltinTestResult] = useState<{
    success?: boolean
    latency_ms?: number
    message?: string
    error?: string
  } | null>(null)

  const refresh = async () => {
    setLoading(true)
    try {
      const [all, libs] = await Promise.all([
        adminAPI.listSettings(),
        libraryAPI.list({ includeHidden: true }).catch(() => [] as Library[]),
      ])
      const idx: Record<string, string> = {}
      for (const item of adultSettingsGroup.items) {
        if (item.defaultValue !== undefined) {
          idx[item.key] = item.defaultValue
        }
      }
      for (const s of all as Setting[]) {
        if (s.key.startsWith('adult.')) {
          idx[s.key] = s.value
        }
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

  const setVal = (key: string, value: string) => {
    setValues((prev) => ({ ...prev, [key]: value }))
    setDirty((prev) => new Set(prev).add(key))
  }

  const toggleVal = (key: string) => {
    const current = values[key] === 'true' || values[key] === '1' || values[key] === 'on'
    setVal(key, current ? 'false' : 'true')
  }

  const isChecked = (key: string, def = false) => {
    const val = values[key]
    if (val === undefined) return def
    return val === 'true' || val === '1' || val === 'on'
  }

  const onSave = async (e?: FormEvent) => {
    if (e) e.preventDefault()
    if (dirty.size === 0) return
    setSaving(true)
    try {
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

  const runMetaTubeTest = async () => {
    setTestingMT(true)
    setMtTestResult(null)
    try {
      const res = await adminAPI.testAdultScraper({
        engine: 'metatube',
        server_url: values['adult.scraper.metatube_server'] || 'http://127.0.0.1:7700',
        token: values['adult.scraper.metatube_token'] || '',
      })
      setMtTestResult(res)
      if (res.success) {
        toast.success(`MetaTube 连接成功 (${res.latency_ms ?? 0}ms)`)
      } else {
        toast.error(res.error || 'MetaTube 连接失败')
      }
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        (err as Error).message ??
        '测试失败'
      setMtTestResult({ success: false, error: msg })
      toast.error(msg)
    } finally {
      setTestingMT(false)
    }
  }

  const runBuiltinTest = async () => {
    setTestingBuiltin(true)
    setBuiltinTestResult(null)
    try {
      const res = await adminAPI.testAdultScraper({
        engine: 'builtin',
        javdb_url: values['adult.scraper.builtin_javdb_url'] || 'https://javdb.com',
        javbus_url: values['adult.scraper.builtin_javbus_url'] || '',
        cookie: values['adult.scraper.builtin_cookie'] || '',
      })
      setBuiltinTestResult(res)
      if (res.success) {
        toast.success(`内置刮削源连接成功 (${res.latency_ms ?? 0}ms)`)
      } else {
        toast.error(res.error || '内置源连接失败')
      }
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        (err as Error).message ??
        '测试失败'
      setBuiltinTestResult({ success: false, error: msg })
      toast.error(msg)
    } finally {
      setTestingBuiltin(false)
    }
  }

  const selectedLibraryIDs = parseLibraryIDs(values['adult.library_ids'] || '[]')
  const toggleLibrary = (id: string, checked: boolean) => {
    const next = checked
      ? Array.from(new Set([...selectedLibraryIDs, id]))
      : selectedLibraryIDs.filter((item) => item !== id)
    setVal('adult.library_ids', JSON.stringify(next))
  }

  const engine = values['adult.scraper.engine'] || 'builtin'

  if (loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  return (
    <form onSubmit={onSave} className="space-y-6">
      {/* 1. 全局访问与隔离卡片 */}
      <div className="glass-panel space-y-4">
        <div className="flex items-center gap-3 border-b border-gray-200/60 pb-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-pink-500/10 text-pink-500">
            <Lock size={18} />
          </div>
          <div>
            <h2 className="text-base font-semibold text-ink-600">访问与媒体库隔离控制</h2>
            <p className="text-xs text-sand-500">
              设置全局成人内容显示规则、指定成人影视库与访问 PIN 保护
            </p>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          {/* 主开关 */}
          <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
            <label className="flex cursor-pointer items-start justify-between gap-4">
              <div>
                <span className="block text-sm font-medium text-ink-600">启用成人内容</span>
                <span className="mt-0.5 block text-xs text-sand-500">
                  全局开关。关闭后系统彻底禁用所有成人库与成人刮削；开启后普通用户仍默认隐藏，可在个人资料或 Bot 自行开启。
                </span>
              </div>
              <input
                type="checkbox"
                className="mt-1 h-5 w-5 accent-pink-500"
                checked={isChecked('adult.enabled', true)}
                onChange={() => toggleVal('adult.enabled')}
              />
            </label>
          </div>

          {/* PIN 保护 */}
          <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
            <label className="flex cursor-pointer items-start justify-between gap-4">
              <div>
                <span className="block text-sm font-medium text-ink-600">访问需要 PIN 码保护</span>
                <span className="mt-0.5 block text-xs text-sand-500">
                  开启后，客户端或网页端浏览成人库内容需输入数字 PIN 码
                </span>
              </div>
              <input
                type="checkbox"
                className="mt-1 h-5 w-5 accent-pink-500"
                checked={isChecked('adult.require_pin', false)}
                onChange={() => toggleVal('adult.require_pin')}
              />
            </label>
            {isChecked('adult.require_pin', false) && (
              <div className="mt-3">
                <input
                  type="text"
                  className="input-base text-sm"
                  placeholder="请输入 4-8 位数字 PIN"
                  value={values['adult.pin'] ?? ''}
                  onChange={(e) => setVal('adult.pin', e.target.value)}
                />
              </div>
            )}
          </div>
        </div>

        {/* 成人媒体库绑定 */}
        <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
          <div className="mb-2">
            <span className="block text-sm font-medium text-ink-600">指定成人媒体库</span>
            <span className="text-xs text-sand-500">
              指定哪些媒体库属于成人分类。指定后，搜索、分类及 Emby/Jellyfin 协议都会统一应用隔离与过滤规则。
            </span>
          </div>

          <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {libraries.length === 0 && (
              <div className="col-span-full py-2 text-xs text-sand-500">暂无媒体库</div>
            )}
            {libraries.map((lib) => {
              const checked = selectedLibraryIDs.includes(lib.id)
              return (
                <label
                  key={lib.id}
                  className={
                    'flex cursor-pointer items-start gap-2.5 rounded-xl border p-2.5 transition ' +
                    (checked
                      ? 'border-pink-500/40 bg-pink-500/5'
                      : 'border-gray-200/60 bg-white/80 hover:bg-gray-50')
                  }
                >
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 accent-pink-500"
                    checked={checked}
                    onChange={(e) => toggleLibrary(lib.id, e.target.checked)}
                  />
                  <div className="min-w-0">
                    <span className="block truncate text-xs font-medium text-ink-600">
                      {lib.name}
                    </span>
                    <span className="block truncate font-mono text-[11px] text-sand-500">
                      {lib.path}
                    </span>
                  </div>
                </label>
              )
            })}
          </div>
        </div>
      </div>

      {/* 2. 番号刮削引擎配置卡片 */}
      <div className="glass-panel space-y-5">
        <div className="flex items-center gap-3 border-b border-gray-200/60 pb-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary-500/10 text-brand-500">
            <Sparkles size={18} />
          </div>
          <div>
            <h2 className="text-base font-semibold text-ink-600">番号刮削配置模块</h2>
            <p className="text-xs text-sand-500">
              支持切换本项目内置番号刮削源或接入 MetaTube 独立服务端，进行精准影视与女优元数据抓取
            </p>
          </div>
        </div>

        {/* 引擎模式选择 */}
        <div>
          <label className="mb-2 block text-xs font-medium uppercase tracking-wider text-sand-500">
            选择番号刮削模式
          </label>
          <div className="grid gap-3 sm:grid-cols-3">
            <button
              type="button"
              onClick={() => setVal('adult.scraper.engine', 'builtin')}
              className={
                'flex flex-col items-start gap-1.5 rounded-2xl border p-4 text-left transition ' +
                (engine === 'builtin'
                  ? 'border-brand-500 bg-primary-500/10 ring-1 ring-brand-500'
                  : 'border-gray-200/60 bg-white/50 hover:bg-white/80')
              }
            >
              <div className="flex w-full items-center justify-between">
                <div className="flex items-center gap-2">
                  <Database size={16} className={engine === 'builtin' ? 'text-brand-500' : 'text-sand-500'} />
                  <span className="font-semibold text-sm text-ink-600">本项目内置刮削</span>
                </div>
                {engine === 'builtin' && <CheckCircle2 size={16} className="text-brand-500" />}
              </div>
              <span className="text-xs text-sand-500">
                集成 JavDB / JavBus 多源并发解析，免部署外部服务端，内置年龄绕过
              </span>
            </button>

            <button
              type="button"
              onClick={() => setVal('adult.scraper.engine', 'metatube')}
              className={
                'flex flex-col items-start gap-1.5 rounded-2xl border p-4 text-left transition ' +
                (engine === 'metatube'
                  ? 'border-brand-500 bg-primary-500/10 ring-1 ring-brand-500'
                  : 'border-gray-200/60 bg-white/50 hover:bg-white/80')
              }
            >
              <div className="flex w-full items-center justify-between">
                <div className="flex items-center gap-2">
                  <Server size={16} className={engine === 'metatube' ? 'text-brand-500' : 'text-sand-500'} />
                  <span className="font-semibold text-sm text-ink-600">MetaTube 服务端</span>
                </div>
                {engine === 'metatube' && <CheckCircle2 size={16} className="text-brand-500" />}
              </div>
              <span className="text-xs text-sand-500">
                连接独立 MetaTube Server 容器，支持多数据源聚合、女优头像与高级插件特性
              </span>
            </button>

            <button
              type="button"
              onClick={() => setVal('adult.scraper.engine', 'auto')}
              className={
                'flex flex-col items-start gap-1.5 rounded-2xl border p-4 text-left transition ' +
                (engine === 'auto'
                  ? 'border-brand-500 bg-primary-500/10 ring-1 ring-brand-500'
                  : 'border-gray-200/60 bg-white/50 hover:bg-white/80')
              }
            >
              <div className="flex w-full items-center justify-between">
                <div className="flex items-center gap-2">
                  <Cpu size={16} className={engine === 'auto' ? 'text-brand-500' : 'text-sand-500'} />
                  <span className="font-semibold text-sm text-ink-600">智能混合模式</span>
                </div>
                {engine === 'auto' && <CheckCircle2 size={16} className="text-brand-500" />}
              </div>
              <span className="text-xs text-sand-500">
                优先请求 MetaTube 服务端，若无匹配或服务离线则自动回退至内置多源
              </span>
            </button>
          </div>
        </div>

        {/* 3. MetaTube 专属配置模块 */}
        {(engine === 'metatube' || engine === 'auto') && (
          <div className="rounded-2xl border border-primary-500/20 bg-primary-500/5 p-4 space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Server className="h-4 w-4 text-brand-500" />
                <span className="text-sm font-semibold text-ink-600">MetaTube 后端配置</span>
              </div>
              <a
                href="https://github.com/metatube-community/metatube-server"
                target="_blank"
                rel="noreferrer"
                className="text-xs text-brand-500 hover:underline flex items-center gap-1"
              >
                <Globe size={12} /> MetaTube 官方文档
              </a>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-ink-100">
                  MetaTube 服务端地址 (Server URL)
                </label>
                <input
                  type="text"
                  className="input-base mt-1 text-sm font-mono"
                  placeholder="http://127.0.0.1:7700"
                  value={values['adult.scraper.metatube_server'] ?? 'http://127.0.0.1:7700'}
                  onChange={(e) => setVal('adult.scraper.metatube_server', e.target.value)}
                />
                <p className="mt-1 text-[11px] text-sand-500">
                  独立部署的 MetaTube Server 地址，如 http://127.0.0.1:7700 或内网 Docker IP
                </p>
              </div>

              <div>
                <label className="block text-xs font-medium text-ink-100">
                  认证 Token (API Token)
                </label>
                <div className="relative mt-1">
                  <input
                    type={showToken ? 'text' : 'password'}
                    className="input-base pr-10 text-sm font-mono"
                    placeholder="留空表示服务端未启用 Token 保护"
                    value={values['adult.scraper.metatube_token'] ?? ''}
                    onChange={(e) => setVal('adult.scraper.metatube_token', e.target.value)}
                  />
                  <button
                    type="button"
                    onClick={() => setShowToken(!showToken)}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-sand-500 hover:text-ink-600"
                  >
                    {showToken ? <EyeOff size={14} /> : <Eye size={14} />}
                  </button>
                </div>
                <p className="mt-1 text-[11px] text-sand-500">
                  MetaTube 服务端配置的 TOKEN 环境变量
                </p>
              </div>
            </div>

            {/* Provider 选择器 */}
            <div>
              <label className="block text-xs font-medium text-ink-100">
                首选 / 默认 Provider 数据源
              </label>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {POPULAR_METATUBE_PROVIDERS.map((p) => {
                  const active = (values['adult.scraper.metatube_provider'] ?? '') === p.id
                  return (
                    <button
                      key={p.id}
                      type="button"
                      onClick={() => setVal('adult.scraper.metatube_provider', p.id)}
                      className={
                        'rounded-lg px-2.5 py-1 text-xs transition ' +
                        (active
                          ? 'bg-primary-500 text-white font-medium shadow-sm'
                          : 'bg-white/80 text-ink-100 border border-gray-200/60 hover:bg-sand-100/50')
                      }
                    >
                      {p.label}
                    </button>
                  )
                })}
              </div>
            </div>

            {/* 测试 MetaTube 连接按钮 */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-primary-500/20 pt-3">
              <button
                type="button"
                disabled={testingMT}
                onClick={runMetaTubeTest}
                className="inline-flex items-center gap-1.5 rounded-xl border border-brand-500/40 bg-brand-500/10 px-3 py-1.5 text-xs font-medium text-brand-500 transition hover:bg-brand-500/20 disabled:opacity-50"
              >
                {testingMT ? <Loader2 size={13} className="animate-spin" /> : <Activity size={13} />}
                测试 MetaTube 连接
              </button>

              {mtTestResult && (
                <div className="flex items-center gap-2 text-xs">
                  {mtTestResult.success ? (
                    <span className="inline-flex items-center gap-1 text-emerald-600 font-medium">
                      <CheckCircle2 size={14} /> 连接成功 ({mtTestResult.latency_ms}ms)
                      {mtTestResult.providers && mtTestResult.providers.length > 0 && (
                        <span className="text-sand-500 font-normal">
                          · 支持 {mtTestResult.providers.length} 个源
                        </span>
                      )}
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1 text-red-500 font-medium">
                      <XCircle size={14} /> {mtTestResult.error}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {/* 4. 本地内置刮削源专属配置 */}
        {(engine === 'builtin' || engine === 'auto') && (
          <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4 space-y-4">
            <div className="flex items-center gap-2">
              <Database className="h-4 w-4 text-brand-500" />
              <span className="text-sm font-semibold text-ink-600">内置多源刮削配置</span>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-ink-100">
                  JavDB 主源 URL
                </label>
                <input
                  type="text"
                  className="input-base mt-1 text-sm font-mono"
                  placeholder="https://javdb.com"
                  value={values['adult.scraper.builtin_javdb_url'] ?? 'https://javdb.com'}
                  onChange={(e) => setVal('adult.scraper.builtin_javdb_url', e.target.value)}
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-ink-100">
                  年龄验证 Cookie 凭据
                </label>
                <input
                  type="text"
                  className="input-base mt-1 text-sm font-mono"
                  placeholder="age=verified; existmag=all"
                  value={values['adult.scraper.builtin_cookie'] ?? 'age=verified; existmag=all'}
                  onChange={(e) => setVal('adult.scraper.builtin_cookie', e.target.value)}
                />
              </div>

              <div className="col-span-full">
                <label className="block text-xs font-medium text-ink-100">
                  JavBus 备用镜像源列表 (换行或逗号分隔)
                </label>
                <textarea
                  rows={2}
                  className="input-base mt-1 text-xs font-mono"
                  placeholder={'https://javbus.sbs\nhttps://www.javbus.com\nhttps://www.cdnbus.cyou'}
                  value={
                    values['adult.scraper.builtin_javbus_url'] ??
                    'https://javbus.sbs,https://www.javbus.com,https://www.cdnbus.cyou,https://www.javsee.cyou,https://www.busjav.cyou'
                  }
                  onChange={(e) => setVal('adult.scraper.builtin_javbus_url', e.target.value)}
                />
              </div>
            </div>

            {/* 测试内置源按钮 */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200/60 pt-3">
              <button
                type="button"
                disabled={testingBuiltin}
                onClick={runBuiltinTest}
                className="inline-flex items-center gap-1.5 rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-ink-600 transition hover:bg-gray-50 disabled:opacity-50"
              >
                {testingBuiltin ? (
                  <Loader2 size={13} className="animate-spin" />
                ) : (
                  <Activity size={13} />
                )}
                测试内置源连接
              </button>

              {builtinTestResult && (
                <div className="flex items-center gap-2 text-xs">
                  {builtinTestResult.success ? (
                    <span className="inline-flex items-center gap-1 text-emerald-600 font-medium">
                      <CheckCircle2 size={14} /> 连接成功 ({builtinTestResult.latency_ms}ms)
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1 text-red-500 font-medium">
                      <XCircle size={14} /> {builtinTestResult.error}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {/* 5. 通用高级刮削选项 */}
        <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4 space-y-3">
          <span className="block text-sm font-semibold text-ink-600">元数据与图片增强选项</span>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-gray-200/60 bg-white/80 p-3 hover:bg-gray-50">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 accent-primary-500"
                checked={isChecked('adult.scraper.crop_cover', true)}
                onChange={() => toggleVal('adult.scraper.crop_cover')}
              />
              <div>
                <span className="block text-xs font-semibold text-ink-600">封面智能裁剪海报</span>
                <span className="mt-0.5 block text-[11px] text-sand-500">
                  自动将 DVD 宽版封套裁剪为正面竖版海报，并保留完整封套作为背景大图
                </span>
              </div>
            </label>

            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-gray-200/60 bg-white/80 p-3 hover:bg-gray-50">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 accent-primary-500"
                checked={isChecked('adult.scraper.enable_actor', true)}
                onChange={() => toggleVal('adult.scraper.enable_actor')}
              />
              <div>
                <span className="block text-xs font-semibold text-ink-600">刮削女优详情与头像</span>
                <span className="mt-0.5 block text-[11px] text-sand-500">
                  抓取女优名字、别名、头像并自动聚合至媒体元数据与分类标签
                </span>
              </div>
            </label>

            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-gray-200/60 bg-white/80 p-3 hover:bg-gray-50">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 accent-primary-500"
                checked={isChecked('adult.scraper.enable_trailer', false)}
                onChange={() => toggleVal('adult.scraper.enable_trailer')}
              />
              <div>
                <span className="block text-xs font-semibold text-ink-600">刮削预告片视频</span>
                <span className="mt-0.5 block text-[11px] text-sand-500">
                  尝试解析官方样片/预告片并保存链接
                </span>
              </div>
            </label>

            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-gray-200/60 bg-white/80 p-3 hover:bg-gray-50">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 accent-primary-500"
                checked={isChecked('adult.scraper.badge', false)}
                onChange={() => toggleVal('adult.scraper.badge')}
              />
              <div>
                <span className="block text-xs font-semibold text-ink-600">添加无码/角标标记</span>
                <span className="mt-0.5 block text-[11px] text-sand-500">
                  为无码破解或流出作品添加识别角标
                </span>
              </div>
            </label>
          </div>
        </div>

        {/* 底部保存条 */}
        <div className="flex items-center justify-between pt-2">
          <span className="text-xs text-sand-500">
            {dirty.size > 0 ? `有 ${dirty.size} 项修改未保存` : '所有设置已是最新'}
          </span>
          <button
            type="submit"
            disabled={saving || dirty.size === 0}
            className="neon-button disabled:opacity-50"
          >
            {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
            保存配置
          </button>
        </div>
      </div>
    </form>
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
