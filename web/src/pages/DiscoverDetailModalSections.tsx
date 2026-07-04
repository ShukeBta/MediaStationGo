import { Download, Rss, X } from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { discoverItemMetaText, type DiscoverSubscriptionForm } from './discoverDetailModalModel'

type FormPatch = Partial<DiscoverSubscriptionForm>

type FieldOption = {
  label: string
  value: string
}

const searchModeOptions: FieldOption[] = [
  { value: 'keyword', label: '标题关键词' },
  { value: 'imdb', label: 'IMDB ID' },
]

const mediaTypeOptions: FieldOption[] = [
  { value: '', label: '自动识别' },
  { value: 'movie', label: '电影' },
  { value: 'tv', label: '电视剧' },
  { value: 'anime', label: '动漫' },
  { value: 'variety', label: '综艺' },
]

const resolutionOptions: FieldOption[] = [
  { value: 'best', label: '自动择优' },
  { value: '2160p', label: '2160p / 4K' },
  { value: '1080p', label: '1080p' },
  { value: '720p', label: '720p' },
]

const qualityOptions: FieldOption[] = [
  { value: '', label: '不限' },
  { value: 'remux', label: 'REMUX' },
  { value: 'bluray', label: 'BluRay' },
  { value: 'web-dl', label: 'WEB-DL' },
  { value: 'hdtv', label: 'HDTV' },
]

const washPriorityOptions: FieldOption[] = [
  { value: 'balanced', label: '均衡' },
  { value: 'resolution', label: '分辨率优先' },
  { value: 'quality', label: '片源质量优先' },
  { value: 'effects', label: 'HDR/DV/Atmos 优先' },
  { value: 'seeders', label: '做种数优先' },
]

export function DiscoverModalHeader({ item, source, onClose }: { item: DiscoverItem; source: string; onClose: () => void }) {
  return (
    <div className="mb-4 flex items-start justify-between gap-3">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-brand-500">{source}</p>
        <h2 className="font-display text-2xl font-bold text-ink-600">{item.title}</h2>
        <p className="mt-1 text-sm text-sand-500">{discoverItemMetaText(item)}</p>
      </div>
      <button className="rounded-full border border-gray-200 p-2 text-ink-50 hover:bg-gray-50" onClick={onClose}>
        <X size={18} />
      </button>
    </div>
  )
}

export function DiscoverArtworkPanel({ item }: { item: DiscoverItem }) {
  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-2xl bg-gray-100">
        {item.poster_url ? (
          <img src={imageURL(item.poster_url)} alt={item.title} className="aspect-[2/3] w-full object-cover" />
        ) : (
          <div className="flex aspect-[2/3] items-center justify-center text-sand-500">无海报</div>
        )}
      </div>
      {item.backdrop_url && (
        <img src={imageURL(item.backdrop_url)} alt="" className="h-24 w-full rounded-2xl object-cover" />
      )}
    </div>
  )
}

export function DiscoverOverviewPanel({ overview }: { overview?: string }) {
  return (
    <section className="rounded-2xl border border-gray-200 bg-gray-50 p-4">
      <h3 className="mb-2 font-semibold text-ink-600">简介</h3>
      <p className="text-sm leading-6 text-ink-100">{overview || '当前数据源没有返回简介。'}</p>
    </section>
  )
}

export function DiscoverSubscriptionRules({
  form,
  busy,
  onChange,
  onSubmit,
}: {
  form: DiscoverSubscriptionForm
  busy: boolean
  onChange: (patch: FormPatch) => void
  onSubmit: () => void
}) {
  return (
    <section className="rounded-2xl border border-primary-400/20 bg-primary-400/5 p-4">
      <h3 className="mb-3 flex items-center gap-2 font-semibold text-ink-600">
        <Rss size={16} />
        订阅下载规则
      </h3>
      <div className="grid gap-3 md:grid-cols-3">
        <TextField label="搜索关键词" value={form.keyword} onChange={(keyword) => onChange({ keyword })} className="md:col-span-2" />
        <SelectField label="搜索方式" value={form.search_mode} options={searchModeOptions} onChange={(search_mode) => onChange({ search_mode })} />
        <TextField label="IMDB ID" value={form.imdb_id} placeholder="tt1160419" onChange={(imdb_id) => onChange({ imdb_id })} />
        <SelectField label="类型" value={form.media_type} options={mediaTypeOptions} onChange={(media_type) => onChange({ media_type })} />
        <SelectField label="分辨率" value={form.resolution} options={resolutionOptions} onChange={(resolution) => onChange({ resolution })} />
        <SelectField label="质量" value={form.quality} options={qualityOptions} onChange={(quality) => onChange({ quality })} />
        <TextField label="特效 / 音轨" value={form.effects} placeholder="hdr,dolby-vision,atmos" onChange={(effects) => onChange({ effects })} />
        <SelectField label="洗版优先级" value={form.wash_priority} options={washPriorityOptions} disabled={!form.wash_enabled} onChange={(wash_priority) => onChange({ wash_priority })} />
        <CheckboxField label="启用洗版择优" checked={form.wash_enabled} onChange={(wash_enabled) => onChange({ wash_enabled })} />
        <TextField label="发布组" value={form.release_groups} placeholder="如 FRDS,OurTV" onChange={(release_groups) => onChange({ release_groups })} />
        <TextField label="排除词" value={form.exclude_words} onChange={(exclude_words) => onChange({ exclude_words })} />
        <TextField label="分类覆盖" value={form.media_category} placeholder="综艺 / 日番 / 欧美剧" onChange={(media_category) => onChange({ media_category })} />
        <TextField label="保存路径覆盖" value={form.save_path} onChange={(save_path) => onChange({ save_path })} />
      </div>
      <DiscoverSubmitBar busy={busy} runNow={form.run_now} onRunNowChange={(run_now) => onChange({ run_now })} onSubmit={onSubmit} />
    </section>
  )
}

function TextField({ label, value, placeholder, className = '', onChange }: { label: string; value: string; placeholder?: string; className?: string; onChange: (value: string) => void }) {
  return (
    <label className={`text-xs text-sand-500 ${className}`}>
      {label}
      <input className="input-base mt-1" placeholder={placeholder} value={value} onChange={(event) => onChange(event.target.value)} />
    </label>
  )
}

function SelectField({ label, value, options, disabled, onChange }: { label: string; value: string; options: FieldOption[]; disabled?: boolean; onChange: (value: string) => void }) {
  return (
    <label className="text-xs text-sand-500">
      {label}
      <select className="input-base mt-1 disabled:opacity-50" disabled={disabled} value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => (
          <option key={option.value || option.label} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

function CheckboxField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-ink-100">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      {label}
    </label>
  )
}

function DiscoverSubmitBar({ busy, runNow, onRunNowChange, onSubmit }: { busy: boolean; runNow: boolean; onRunNowChange: (value: boolean) => void; onSubmit: () => void }) {
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
      <label className="flex items-center gap-2 text-sm text-ink-100">
        <input type="checkbox" checked={runNow} onChange={(event) => onRunNowChange(event.target.checked)} />
        创建后立即搜索并下载
      </label>
      <button disabled={busy} onClick={onSubmit} className="neon-button disabled:opacity-60">
        <Download size={16} />
        {busy ? '处理中…' : '创建订阅'}
      </button>
    </div>
  )
}
