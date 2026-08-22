import { FormEvent } from 'react'
import { CloudDownload, Plus, Rss, Save } from 'lucide-react'

import type { Library } from '../types'
import type { SubscriptionFormValues } from './subscriptionFormModel'

interface SubscriptionFormProps {
  values: SubscriptionFormValues
  libraries: Library[]
  editing: boolean
  onSubmit: (event: FormEvent) => void
  onCancelEdit: () => void
  onChange: <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => void
}

export function SubscriptionForm({ values, libraries, editing, onSubmit, onCancelEdit, onChange }: SubscriptionFormProps) {
  const resourceMode = values.deliveryMode === 'resource_import'
  const selectedLibrary = libraries.find((library) => library.id === values.libraryID)
  const roots = (selectedLibrary?.roots ?? []).filter((root) => root.enabled)

  const selectLibrary = (libraryID: string) => {
    const library = libraries.find((item) => item.id === libraryID)
    const enabledRoots = (library?.roots ?? []).filter((root) => root.enabled)
    onChange('libraryID', libraryID)
    onChange('libraryRootID', enabledRoots.length === 1 ? enabledRoots[0].id : '')
    if (library?.type === 'anime') onChange('mediaType', 'anime')
    else if (library?.type === 'tv') onChange('mediaType', 'tv')
  }

  return (
    <form onSubmit={onSubmit} className="glass-panel space-y-4">
      <div className="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1">
        <ModeButton
          active={resourceMode}
          icon={CloudDownload}
          label="自动追更"
          onClick={() => onChange('deliveryMode', 'resource_import')}
        />
        <ModeButton
          active={!resourceMode}
          icon={Rss}
          label="RSS / PT"
          onClick={() => onChange('deliveryMode', 'download')}
        />
      </div>

      <div className="grid gap-3 md:grid-cols-4">
        <input
          required
          className="input-base"
          placeholder="作品名称"
          value={values.name}
          onChange={(event) => onChange('name', event.target.value)}
        />
        <input
          className="input-base"
          placeholder={resourceMode ? '搜索关键词（默认使用作品名称）' : '过滤器（正则，可选）'}
          value={values.filter}
          onChange={(event) => onChange('filter', event.target.value)}
        />
        <select className="input-base" value={values.mediaType} onChange={(event) => onChange('mediaType', event.target.value)}>
          {!resourceMode && <option value="">自动识别类型</option>}
          {!resourceMode && <option value="movie">电影</option>}
          <option value="tv">电视剧</option>
          <option value="anime">动漫</option>
          <option value="variety">综艺</option>
        </select>

        {resourceMode ? (
          <>
            <select required className="input-base" value={values.libraryID} onChange={(event) => selectLibrary(event.target.value)}>
              <option value="">选择目标媒体库</option>
              {libraries.map((library) => (
                <option key={library.id} value={library.id}>{library.name}</option>
              ))}
            </select>
            <select
              required
              className="input-base"
              value={values.libraryRootID}
              onChange={(event) => onChange('libraryRootID', event.target.value)}
              disabled={!values.libraryID}
            >
              <option value="">选择入库目录</option>
              {roots.map((root) => (
                <option key={root.id} value={root.id}>{root.name || root.path}</option>
              ))}
            </select>
            <input
              required
              min={1}
              type="number"
              className="input-base"
              placeholder="季数"
              value={values.seasonNumber}
              onChange={(event) => onChange('seasonNumber', event.target.value)}
            />
            <input
              min={0}
              type="number"
              className="input-base"
              placeholder="总集数（未知可留空）"
              value={values.totalEpisodes}
              onChange={(event) => onChange('totalEpisodes', event.target.value)}
            />
            <label className="block text-xs text-sand-500">
              每轮候选上限
              <select className="input-base mt-1" value={values.maxImportsPerRun} onChange={(event) => onChange('maxImportsPerRun', event.target.value)}>
                {[1, 2, 3, 4, 5].map((value) => <option key={value} value={value}>{value} 集</option>)}
              </select>
            </label>
          </>
        ) : (
          <>
            <input
              required
              className="input-base md:col-span-2"
              placeholder="RSS 地址"
              value={values.feed}
              onChange={(event) => onChange('feed', event.target.value)}
            />
            <input
              className="input-base"
              placeholder="二级分类覆盖（可选）"
              value={values.mediaCategory}
              onChange={(event) => onChange('mediaCategory', event.target.value)}
            />
            <input
              className="input-base"
              placeholder="下载根目录覆盖（可选）"
              value={values.savePath}
              onChange={(event) => onChange('savePath', event.target.value)}
            />
            <select className="input-base" value={values.searchMode} onChange={(event) => onChange('searchMode', event.target.value)}>
              <option value="keyword">标题关键词搜索</option>
              <option value="imdb">IMDB ID 搜索</option>
            </select>
            <input className="input-base" placeholder="IMDB ID" value={values.imdbID} onChange={(event) => onChange('imdbID', event.target.value)} />
          </>
        )}

        <label className="block text-xs text-sand-500">
          扫描频率（分钟）
          <input
            required
            min={5}
            max={1440}
            type="number"
            className="input-base mt-1"
            value={values.pollIntervalMinutes}
            onChange={(event) => onChange('pollIntervalMinutes', event.target.value)}
          />
        </label>

        <select className="input-base" value={values.resolution} onChange={(event) => onChange('resolution', event.target.value)}>
          <option value="best">分辨率自动择优</option>
          <option value="2160p">2160p / 4K</option>
          <option value="1080p">1080p</option>
          <option value="720p">720p</option>
        </select>
        <select className="input-base" value={values.quality} onChange={(event) => onChange('quality', event.target.value)}>
          <option value="">质量不限</option>
          <option value="remux">REMUX</option>
          <option value="bluray">BluRay</option>
          <option value="web-dl">WEB-DL</option>
          <option value="hdtv">HDTV</option>
        </select>
        <input className="input-base" placeholder="特效 / 音轨" value={values.effects} onChange={(event) => onChange('effects', event.target.value)} />
        <input className="input-base" placeholder="发布组白名单" value={values.releaseGroups} onChange={(event) => onChange('releaseGroups', event.target.value)} />
        <input className="input-base md:col-span-2" placeholder="排除词（逗号分隔）" value={values.excludeWords} onChange={(event) => onChange('excludeWords', event.target.value)} />

        {!resourceMode && (
          <>
            <input className="input-base" inputMode="numeric" placeholder="最少做种数" value={values.minSeeders} onChange={(event) => onChange('minSeeders', event.target.value)} />
            <input className="input-base" inputMode="numeric" placeholder="最多做种数" value={values.maxSeeders} onChange={(event) => onChange('maxSeeders', event.target.value)} />
            <input className="input-base" inputMode="decimal" placeholder="最小体积 GB" value={values.minSizeGB} onChange={(event) => onChange('minSizeGB', event.target.value)} />
            <input className="input-base" inputMode="decimal" placeholder="最大体积 GB" value={values.maxSizeGB} onChange={(event) => onChange('maxSizeGB', event.target.value)} />
            <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
              <input type="checkbox" checked={values.freeOnly} onChange={(event) => onChange('freeOnly', event.target.checked)} />
              只下载免费资源
            </label>
            <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
              <input type="checkbox" checked={values.washEnabled} onChange={(event) => onChange('washEnabled', event.target.checked)} />
              启用洗版择优
            </label>
          </>
        )}
      </div>

      <div className="flex justify-end gap-2">
        {editing && (
          <button type="button" onClick={onCancelEdit} className="btn-outline">取消</button>
        )}
        <button type="submit" className="neon-button">
          {editing ? <Save size={16} /> : <Plus size={16} />}
          {editing ? '保存' : '创建订阅'}
        </button>
      </div>
    </form>
  )
}

function ModeButton({ active, icon: Icon, label, onClick }: { active: boolean; icon: typeof CloudDownload; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition ${
        active ? 'bg-white text-brand-500 shadow-sm' : 'text-sand-500 hover:text-ink-600'
      }`}
    >
      <Icon size={15} />
      {label}
    </button>
  )
}
