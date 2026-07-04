import { FormEvent } from 'react'
import { Plus, Save } from 'lucide-react'

import type { SubscriptionFormValues } from './subscriptionFormModel'

interface SubscriptionFormProps {
  values: SubscriptionFormValues
  editing: boolean
  onSubmit: (event: FormEvent) => void
  onCancelEdit: () => void
  onChange: <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => void
}

export function SubscriptionForm({ values, editing, onSubmit, onCancelEdit, onChange }: SubscriptionFormProps) {
  return (
    <form onSubmit={onSubmit} className="glass-panel grid gap-3 md:grid-cols-4">
      <input
        required
        className="input-base"
        placeholder="名称(显示用)"
        value={values.name}
        onChange={(e) => onChange('name', e.target.value)}
      />
      <input
        required
        className="input-base"
        placeholder="RSS 地址"
        value={values.feed}
        onChange={(e) => onChange('feed', e.target.value)}
      />
      <input
        className="input-base"
        placeholder="过滤器(正则,可选)"
        value={values.filter}
        onChange={(e) => onChange('filter', e.target.value)}
      />
      <select className="input-base" value={values.mediaType} onChange={(e) => onChange('mediaType', e.target.value)}>
        <option value="">自动识别类型</option>
        <option value="movie">电影</option>
        <option value="tv">电视剧</option>
        <option value="anime">动漫</option>
        <option value="variety">综艺</option>
      </select>
      <input
        className="input-base"
        placeholder="二级分类覆盖(如 综艺/日番,可选)"
        value={values.mediaCategory}
        onChange={(e) => onChange('mediaCategory', e.target.value)}
      />
      <input
        className="input-base md:col-span-2"
        placeholder="下载根目录覆盖(可选,默认使用下载器保存路径)"
        value={values.savePath}
        onChange={(e) => onChange('savePath', e.target.value)}
      />
      <select className="input-base" value={values.searchMode} onChange={(e) => onChange('searchMode', e.target.value)}>
        <option value="keyword">标题关键词搜索</option>
        <option value="imdb">IMDB ID 搜索</option>
      </select>
      <input
        className="input-base"
        placeholder="IMDB ID，如 tt1160419"
        value={values.imdbID}
        onChange={(e) => onChange('imdbID', e.target.value)}
      />
      <select className="input-base" value={values.resolution} onChange={(e) => onChange('resolution', e.target.value)}>
        <option value="best">分辨率自动择优</option>
        <option value="2160p">2160p / 4K</option>
        <option value="1080p">1080p</option>
        <option value="720p">720p</option>
      </select>
      <select className="input-base" value={values.quality} onChange={(e) => onChange('quality', e.target.value)}>
        <option value="">质量不限</option>
        <option value="remux">REMUX</option>
        <option value="bluray">BluRay</option>
        <option value="web-dl">WEB-DL</option>
        <option value="hdtv">HDTV</option>
      </select>
      <input
        className="input-base"
        placeholder="特效/音轨 hdr,dolby-vision,atmos"
        value={values.effects}
        onChange={(e) => onChange('effects', e.target.value)}
      />
      <label className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
        <input
          type="checkbox"
          checked={values.washEnabled}
          onChange={(e) => onChange('washEnabled', e.target.checked)}
        />
        启用洗版择优
      </label>
      <select
        className="input-base disabled:opacity-50"
        disabled={!values.washEnabled}
        value={values.washPriority}
        onChange={(e) => onChange('washPriority', e.target.value)}
      >
        <option value="balanced">洗版：均衡</option>
        <option value="resolution">洗版：分辨率优先</option>
        <option value="quality">洗版：片源质量优先</option>
        <option value="effects">洗版：HDR/DV/Atmos 优先</option>
        <option value="seeders">洗版：做种数优先</option>
      </select>
      <input
        className="input-base"
        placeholder="发布组白名单，如 FRDS,OurTV"
        value={values.releaseGroups}
        onChange={(e) => onChange('releaseGroups', e.target.value)}
      />
      <input
        className="input-base"
        placeholder="排除词，如 cam,ts,dovi,h265,10bit,杜比"
        value={values.excludeWords}
        onChange={(e) => onChange('excludeWords', e.target.value)}
      />
      <input
        className="input-base"
        inputMode="numeric"
        placeholder="最少做种数"
        value={values.minSeeders}
        onChange={(e) => onChange('minSeeders', e.target.value)}
      />
      <input
        className="input-base"
        inputMode="numeric"
        placeholder="最多做种数"
        value={values.maxSeeders}
        onChange={(e) => onChange('maxSeeders', e.target.value)}
      />
      <input
        className="input-base"
        inputMode="decimal"
        placeholder="最小体积 GB"
        value={values.minSizeGB}
        onChange={(e) => onChange('minSizeGB', e.target.value)}
      />
      <input
        className="input-base"
        inputMode="decimal"
        placeholder="最大体积 GB"
        value={values.maxSizeGB}
        onChange={(e) => onChange('maxSizeGB', e.target.value)}
      />
      <label className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
        <input
          type="checkbox"
          checked={values.freeOnly}
          onChange={(e) => onChange('freeOnly', e.target.checked)}
        />
        只下载免费资源
      </label>
      <button type="submit" className="neon-button md:col-span-1">
        {editing ? <Save size={16} /> : <Plus size={16} />}
        {editing ? '保存' : '添加'}
      </button>
      {editing && (
        <button
          type="button"
          onClick={onCancelEdit}
          className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:bg-gray-50"
        >
          取消编辑
        </button>
      )}
    </form>
  )
}
