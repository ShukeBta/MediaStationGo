import type { SubscriptionRuleFormValues } from './subscriptionDefaultRulesModel'
import type { SubscriptionFormValues } from './subscriptionFormModel'

export function SubscriptionRuleFields({ values, onChange, resourceMode = false, disabled = false }: {
  values: SubscriptionRuleFormValues
  onChange: <K extends keyof SubscriptionRuleFormValues>(key: K, value: SubscriptionFormValues[K]) => void
  resourceMode?: boolean
  disabled?: boolean
}) {
  return (
    <fieldset disabled={disabled} className="grid gap-3 md:grid-cols-4 disabled:opacity-60">
      <label className="block text-xs text-sand-500">
        扫描频率（分钟）
        <input required min={5} max={1440} type="number" className="input-base mt-1" value={values.pollIntervalMinutes} onChange={(event) => onChange('pollIntervalMinutes', event.target.value)} />
      </label>
      <label className="block text-xs text-sand-500">
        分辨率
        <select aria-label="分辨率" className="input-base mt-1" value={values.resolution} onChange={(event) => onChange('resolution', event.target.value)}>
          <option value="best">自动择优</option>
          <option value="2160p">2160p / 4K</option>
          <option value="1080p">1080p</option>
          <option value="720p">720p</option>
        </select>
      </label>
      <label className="block text-xs text-sand-500">
        质量
        <select aria-label="质量" className="input-base mt-1" value={values.quality} onChange={(event) => onChange('quality', event.target.value)}>
          <option value="">不限</option>
          <option value="best">自动择优</option>
          <option value="remux">REMUX</option>
          <option value="bluray">BluRay</option>
          <option value="web-dl">WEB-DL</option>
          <option value="hdtv">HDTV</option>
        </select>
      </label>
      <label className="block text-xs text-sand-500">特效 / 音轨<input className="input-base mt-1" maxLength={128} value={values.effects} onChange={(event) => onChange('effects', event.target.value)} /></label>
      <label className="block text-xs text-sand-500">发布组白名单<input className="input-base mt-1" maxLength={255} value={values.releaseGroups} onChange={(event) => onChange('releaseGroups', event.target.value)} /></label>
      <label className="block text-xs text-sand-500 md:col-span-3">排除词（逗号分隔）<input className="input-base mt-1" maxLength={255} value={values.excludeWords} onChange={(event) => onChange('excludeWords', event.target.value)} /></label>
      {!resourceMode && <>
        <label className="block text-xs text-sand-500">最少做种数<input type="number" min={0} className="input-base mt-1" placeholder="不限" value={values.minSeeders} onChange={(event) => onChange('minSeeders', event.target.value)} /></label>
        <label className="block text-xs text-sand-500">最多做种数<input type="number" min={0} className="input-base mt-1" placeholder="不限" value={values.maxSeeders} onChange={(event) => onChange('maxSeeders', event.target.value)} /></label>
        <label className="block text-xs text-sand-500">最小体积（GB）<input type="number" min={0} step="any" className="input-base mt-1" placeholder="不限" value={values.minSizeGB} onChange={(event) => onChange('minSizeGB', event.target.value)} /></label>
        <label className="block text-xs text-sand-500">最大体积（GB）<input type="number" min={0} step="any" className="input-base mt-1" placeholder="不限" value={values.maxSizeGB} onChange={(event) => onChange('maxSizeGB', event.target.value)} /></label>
        <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
          <input type="checkbox" checked={values.freeOnly} onChange={(event) => onChange('freeOnly', event.target.checked)} />只下载免费资源
        </label>
        <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-100">
          <input type="checkbox" checked={values.washEnabled} onChange={(event) => onChange('washEnabled', event.target.checked)} />启用洗版择优
        </label>
        <label className="block text-xs text-sand-500 md:col-span-2">
          洗版优先级
          <select aria-label="洗版优先级" className="input-base mt-1" value={values.washPriority} onChange={(event) => onChange('washPriority', event.target.value)}>
            <option value="balanced">综合择优</option>
            <option value="resolution">分辨率优先</option>
            <option value="quality">质量优先</option>
            <option value="effects">特效 / 音轨优先</option>
            <option value="seeders">做种数优先</option>
          </select>
        </label>
      </>}
    </fieldset>
  )
}
