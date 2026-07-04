import { Loader2, Save, Wand2 } from 'lucide-react'

import type { GenerateSTRMResult, STRMRefreshResult } from '../api/strm'
import type { Library } from '../types'
import { currentOrigin, type CloudPlaybackMode } from './strmPageModel'
import type { StrmGenerateSectionProps } from './StrmGenerateSection'
import { StrmOutputDirPicker } from './StrmOutputDirPicker'

type PlaybackStatusProps = Pick<
  StrmGenerateSectionProps,
  'playbackStatus' | 'strmPlaybackEnabled' | 'redirectProxyEnabled'
>

export function StrmGenerateHeader({ playbackStatus, strmPlaybackEnabled, redirectProxyEnabled }: PlaybackStatusProps) {
  const enabled = strmPlaybackEnabled || redirectProxyEnabled
  return (
    <div className="flex items-start justify-between gap-3">
      <div>
        <h2 className="font-display text-lg font-semibold text-ink-600">自动生成 STRM 文件</h2>
        <p className="text-sm text-ink-50">
          只需要填写自己的访问域名，系统会按媒体库内每个媒体批量生成可播放的 .strm 文件。
        </p>
      </div>
      <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${
        enabled
          ? 'border-emerald-300/40 bg-emerald-400/10 text-emerald-500'
          : 'border-red-300/40 bg-red-400/10 text-red-500'
      }`}>
        {playbackStatus}
      </span>
    </div>
  )
}

type PlaybackToggleProps = Pick<
  StrmGenerateSectionProps,
  'strmPlaybackEnabled' | 'redirectProxyEnabled' | 'setStrmPlaybackEnabled' | 'setRedirectProxyEnabled'
>

export function PlaybackTogglePanel({
  strmPlaybackEnabled,
  redirectProxyEnabled,
  setStrmPlaybackEnabled,
  setRedirectProxyEnabled,
}: PlaybackToggleProps) {
  return (
    <div className="grid gap-3 rounded-2xl border border-gray-200 bg-white/70 p-4 md:grid-cols-2">
      <PlaybackToggle
        checked={strmPlaybackEnabled}
        title="启用 STRMURL 播放"
        description="第三方客户端可拿到 /api/stream/媒体ID 入口，适合 STRM 管理和自动生成方案。"
        onChange={setStrmPlaybackEnabled}
      />
      <PlaybackToggle
        checked={redirectProxyEnabled}
        title="启用 302/反代播放"
        description="第三方客户端可拿到 /Videos/媒体ID/stream 入口，由服务端解析后 302 或必要时反代。"
        onChange={setRedirectProxyEnabled}
      />
    </div>
  )
}

type PlaybackToggleItemProps = {
  checked: boolean
  title: string
  description: string
  onChange: (value: boolean) => void
}

function PlaybackToggle({ checked, title, description, onChange }: PlaybackToggleItemProps) {
  return (
    <label className="flex items-start gap-3 text-sm text-ink-100">
      <input
        type="checkbox"
        className="mt-1 h-4 w-4 accent-primary-400"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        <span className="block font-medium text-ink-600">{title}</span>
        <span className="text-xs text-ink-50">{description}</span>
      </span>
    </label>
  )
}

type PlaybackPreferenceProps = Pick<
  StrmGenerateSectionProps,
  | 'cloudPlaybackMode'
  | 'strmPlaybackEnabled'
  | 'redirectProxyEnabled'
  | 'autoGenerate'
  | 'savingSettings'
  | 'saveSTRMSettings'
  | 'setCloudPlaybackMode'
  | 'setAutoGenerate'
>

export function PlaybackPreferencePanel({
  cloudPlaybackMode,
  strmPlaybackEnabled,
  redirectProxyEnabled,
  autoGenerate,
  savingSettings,
  saveSTRMSettings,
  setCloudPlaybackMode,
  setAutoGenerate,
}: PlaybackPreferenceProps) {
  return (
    <div className="grid gap-3 rounded-2xl border border-gray-200 bg-white/70 p-4 md:grid-cols-[1fr_1fr_auto]">
      <label className="text-sm text-ink-100">
        <span className="mb-1 block font-medium text-ink-600">两者都开启时优先</span>
        <select
          className="input-base"
          value={cloudPlaybackMode}
          onChange={(e) => setCloudPlaybackMode(e.target.value as CloudPlaybackMode)}
          disabled={!strmPlaybackEnabled || !redirectProxyEnabled}
        >
          <option value="strm">优先 STRMURL</option>
          <option value="redirect_proxy">优先 302/反代</option>
        </select>
        <span className="mt-1 block text-xs text-ink-50">只开启一个时自动使用已开启的播放方式；两个都关闭时云盘媒体不向第三方提供播放。</span>
      </label>
      <PlaybackToggle
        checked={autoGenerate}
        title="扫描后自动刷新 STRM 文件"
        description="默认关闭，避免扫描大型网盘库时重复写文件。"
        onChange={setAutoGenerate}
      />
      <button type="button" className="neon-button self-center" disabled={savingSettings} onClick={saveSTRMSettings}>
        {savingSettings ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
        保存开关
      </button>
    </div>
  )
}

type StrmGenerateFormProps = Pick<
  StrmGenerateSectionProps,
  | 'libraries'
  | 'generateLibraryID'
  | 'baseURL'
  | 'outputDir'
  | 'outputPresets'
  | 'overwrite'
  | 'includeLocal'
  | 'preserveTree'
  | 'refreshLibrary'
  | 'scrapeAfter'
  | 'generating'
  | 'onGenerate'
  | 'setGenerateLibraryID'
  | 'setBaseURL'
  | 'setOutputDir'
  | 'setOverwrite'
  | 'setIncludeLocal'
  | 'setPreserveTree'
  | 'setRefreshLibrary'
  | 'setScrapeAfter'
>

export function StrmGenerateForm({
  libraries,
  generateLibraryID,
  baseURL,
  outputDir,
  outputPresets,
  overwrite,
  includeLocal,
  preserveTree,
  refreshLibrary,
  scrapeAfter,
  generating,
  onGenerate,
  setGenerateLibraryID,
  setBaseURL,
  setOutputDir,
  setOverwrite,
  setIncludeLocal,
  setPreserveTree,
  setRefreshLibrary,
  setScrapeAfter,
}: StrmGenerateFormProps) {
  return (
    <form onSubmit={onGenerate} className="grid gap-3 md:grid-cols-4">
      <LibrarySelect libraries={libraries} value={generateLibraryID} onChange={setGenerateLibraryID} />
      <input
        required
        className="input-base md:col-span-2"
        placeholder="http://NAS-IP:18080 或 https://media.example.com"
        value={baseURL}
        onChange={(e) => setBaseURL(e.target.value)}
      />
      <button
        type="button"
        className="rounded-2xl border border-primary-400/40 px-3 py-2 text-sm text-brand-500 transition hover:bg-primary-400/10"
        onClick={() => setBaseURL(currentOrigin())}
      >
        使用当前访问地址
      </button>
      <div className="grid gap-2 md:col-span-4 md:grid-cols-5">
        <CompactOption checked={overwrite} label="覆盖已存在" onChange={setOverwrite} />
        <CompactOption checked={includeLocal} label="包含本地媒体" onChange={setIncludeLocal} />
        <CompactOption checked={preserveTree} label="保留目录树" onChange={setPreserveTree} />
        <CompactOption checked={refreshLibrary} label="生成后刷新媒体库" onChange={setRefreshLibrary} />
        <CompactOption
          checked={refreshLibrary && scrapeAfter}
          disabled={!refreshLibrary}
          label="刷新后自动刮削"
          onChange={setScrapeAfter}
        />
      </div>
      <StrmOutputDirPicker
        className="md:col-span-4"
        presets={outputPresets}
        placeholder="输出目录可留空，默认写入 data/strm/分类/子分类"
        value={outputDir}
        onChange={setOutputDir}
      />
      <button type="submit" disabled={generating || !generateLibraryID || !baseURL.trim()} className="neon-button md:col-span-4">
        {generating ? <Loader2 size={16} className="animate-spin" /> : <Wand2 size={16} />}
        {generating ? '生成中…' : '批量生成 STRM'}
      </button>
    </form>
  )
}

type CompactOptionProps = {
  checked: boolean
  disabled?: boolean
  label: string
  onChange: (value: boolean) => void
}

function CompactOption({ checked, disabled, label, onChange }: CompactOptionProps) {
  return (
    <label className={`flex min-h-10 items-center gap-2 rounded-2xl border border-gray-200 bg-white/70 px-3 py-2 text-sm text-ink-50 ${disabled ? 'cursor-not-allowed opacity-50' : ''}`}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label}
    </label>
  )
}

type LibrarySelectProps = {
  libraries: Library[]
  value: string
  onChange: (value: string) => void
}

function LibrarySelect({ libraries, value, onChange }: LibrarySelectProps) {
  return (
    <select
      required
      className="input-base"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="" disabled>
        选择媒体库
      </option>
      <option value="*">全部媒体库</option>
      {libraries.map((library) => (
        <option key={library.id} value={library.id}>
          {library.name} ({library.type})
        </option>
      ))}
    </select>
  )
}

export function StrmGenerateHint() {
  return (
    <p className="text-xs text-sand-500">
      生成内容为 <code>域名 + /api/stream/媒体ID?token=...</code>；第三方客户端播放优先方式由上方「STRMURL / 302反代」模式决定。域名会同步保存到系统设置中的「公开访问域名 / STRM 域名」。
    </p>
  )
}

export function StrmGenerateResultPanel({ result }: { result: GenerateSTRMResult | null }) {
  if (!result) {
    return null
  }
  return (
    <div className="rounded-2xl border border-gray-200 bg-gray-50 p-4 text-sm text-ink-50">
      <div className="font-semibold text-ink-600">
        输出目录：{result.output_dir}
      </div>
      <div className="mt-1">
        {result.previewed ? `预检 ${result.previewed} · ` : ''}
        {result.total ? `共 ${result.total} · ` : ''}
        新增 {result.generated} · 更新 {result.updated} · 跳过 {result.skipped}
        {result.ignored ? ` · 忽略 ${result.ignored}` : ''} · 清理 {result.cleaned || 0}
      </div>
      {result.batch_limited && (
        <div className="mt-1 text-amber-600">本批已达到上限，剩余 {result.remaining || 0} 个可继续运行。</div>
      )}
      <StrmRefreshStatus refresh={result.refresh} />
      {result.ignored_items && result.ignored_items.length > 0 && (
        <div className="mt-1 text-amber-600">
          已忽略 {result.ignored || result.ignored_items.length} 个非视频/sidecar：{result.ignored_items.slice(0, 3).join('；')}
        </div>
      )}
      {result.errors && result.errors.length > 0 && (
        <div className="mt-2 text-red-500">
          失败 {result.errors.length} 条：{result.errors.slice(0, 3).join('；')}
        </div>
      )}
    </div>
  )
}

export function StrmRefreshStatus({ refresh }: { refresh?: STRMRefreshResult }) {
  if (!refresh) {
    return null
  }
  return (
    <>
      <div className={refresh.queued ? 'mt-1 text-emerald-600' : 'mt-1 text-amber-600'}>
        {refresh.queued
          ? `媒体库刷新已排队：${refresh.targets?.map((target) => target.name).join('、') || '已匹配媒体库'}`
          : `媒体库未刷新：${refresh.reason || '未匹配到可扫描媒体库'}`}
      </div>
      {refresh.scrape_requested && (
        <div className={refresh.scrape_queued ? 'mt-1 text-emerald-600' : 'mt-1 text-amber-600'}>
          {refresh.scrape_queued
            ? '扫描完成后将自动刮削'
            : `未安排刮削：${refresh.scrape_reason || '媒体库未刷新'}`}
        </div>
      )}
    </>
  )
}
