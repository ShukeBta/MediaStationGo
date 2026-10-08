import { FormEvent } from 'react'
import { CloudDownload, Loader2, Plus, Rss, Save, Search } from 'lucide-react'

import type { Library } from '../types'
import type { SubscriptionFormValues } from './subscriptionFormModel'
import { LocalDownloadPathField } from './LocalDownloadPathField'
import { useResourceImportCapability } from '../hooks/useResourceImportCapability'
import { CloudImportAvailability } from './CloudImportAvailability'
import { SubscriptionRuleFields } from './SubscriptionRuleFields'

interface SubscriptionFormProps {
  values: SubscriptionFormValues
  libraries: Library[]
  editing: boolean
  busy: boolean
  onSubmit: (event: FormEvent) => void
  onCancelEdit: () => void
  onChange: <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => void
  onModeChange: (delivery: SubscriptionFormValues['deliveryMode'], source: SubscriptionFormValues['sourceMode']) => void
}

export function SubscriptionForm({ values, libraries, editing, busy, onSubmit, onCancelEdit, onChange, onModeChange }: SubscriptionFormProps) {
  const resourceMode = values.deliveryMode === 'resource_import'
  const cloud = useResourceImportCapability()
  const ptMode = !resourceMode && values.sourceMode === 'pt'
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
      <div className="inline-flex flex-wrap rounded-lg border border-gray-200 bg-gray-50 p-1">
        <ModeButton
          active={ptMode}
          icon={Search}
          label="PT 自动追更"
          onClick={() => onModeChange('download', 'pt')}
        />
        <ModeButton
          active={resourceMode}
          icon={CloudDownload}
          label="网盘入库"
          onClick={() => onModeChange('resource_import', 'pt')}
        />
        <ModeButton
          active={!resourceMode && !ptMode}
          icon={Rss}
          label="RSS 订阅"
          onClick={() => onModeChange('download', 'rss')}
        />
      </div>
      <p className="text-xs text-sand-500">{ptMode ? '使用已配置 PT 站点按关键词持续搜索，创建后立即执行一次，再按计划下载符合规则的新资源。' : resourceMode ? '需要已配置的网盘入库服务及目标云盘媒体库。' : '定期读取 RSS 地址，下载符合过滤规则的新资源。'}</p>
      {resourceMode && <CloudImportAvailability {...cloud} />}
      {!resourceMode && <LocalDownloadPathField value={values.savePath} onChange={(value) => onChange('savePath', value)} disabled={busy} canManage subscription />}

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
          placeholder={resourceMode ? '搜索关键词（默认使用作品名称）' : ptMode ? '备用搜索关键词（可选）' : '过滤器（正则，可选）'}
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

        {resourceMode ? cloud.state === 'enabled' ? (
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
        ) : null : (
          <>
            {ptMode ? <input
              className="input-base md:col-span-2"
              placeholder="PT 搜索关键词（默认使用作品名称）"
              value={values.searchKeyword}
              onChange={(event) => onChange('searchKeyword', event.target.value)}
            /> : <input
              required
              className="input-base md:col-span-2"
              placeholder="RSS 地址"
              value={values.feed}
              onChange={(event) => onChange('feed', event.target.value)}
            />}
            {ptMode && <>
              <label className="text-xs text-sand-500">季数<input min={1} type="number" className="input-base mt-1" value={values.seasonNumber} onChange={(event) => onChange('seasonNumber', event.target.value)} /></label>
              <label className="text-xs text-sand-500">总集数<input min={0} type="number" className="input-base mt-1" placeholder="未知可留空" value={values.totalEpisodes} onChange={(event) => onChange('totalEpisodes', event.target.value)} /></label>
            </>}
            <input
              className="input-base"
              placeholder="二级分类覆盖（可选）"
              value={values.mediaCategory}
              onChange={(event) => onChange('mediaCategory', event.target.value)}
            />
            <select className="input-base" value={values.searchMode} onChange={(event) => onChange('searchMode', event.target.value)}>
              <option value="keyword">标题关键词搜索</option>
              <option value="imdb">IMDB ID 搜索</option>
            </select>
            <input className="input-base" placeholder="IMDB ID" value={values.imdbID} onChange={(event) => onChange('imdbID', event.target.value)} />
          </>
        )}

      </div>

      <SubscriptionRuleFields values={values} onChange={onChange} resourceMode={resourceMode} disabled={busy} />

      <div className="flex justify-end gap-2">
        {editing && (
          <button type="button" onClick={onCancelEdit} disabled={busy} className="btn-outline">取消</button>
        )}
        <button type="submit" className="neon-button disabled:cursor-not-allowed disabled:opacity-60" disabled={busy || (resourceMode && cloud.state !== 'enabled')}>
          {busy ? <Loader2 size={16} className="animate-spin" /> : editing ? <Save size={16} /> : <Plus size={16} />}
          {busy ? '提交中…' : editing ? '保存' : '创建订阅'}
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
