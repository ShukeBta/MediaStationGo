import {
  type AutoOrganizeConfig,
  settingOn,
} from './autoOrganizeModel'

type ConfigChangeHandler = (key: keyof AutoOrganizeConfig, value: string) => void

type AutoOrganizeTabProps = {
  config: AutoOrganizeConfig
  onConfigChange: ConfigChangeHandler
}

export function AutoOrganizeBasicTab({
  config,
  currentDir,
  moveKeepsSeeding,
  onConfigChange,
}: AutoOrganizeTabProps & {
  currentDir: string
  moveKeepsSeeding: boolean
}) {
  return (
    <>
      <div className="grid gap-3 lg:grid-cols-[1fr_1fr_150px_140px]">
        <label className="space-y-1">
          <span className="text-xs text-ink-50">整理源目录（待整理 / 下载目录）</span>
          <div className="flex gap-2">
            <input
              className="input-base w-full"
              placeholder="例如 F:\\downloads 或 /downloads"
              value={config.sourceDir}
              onChange={(event) => onConfigChange('sourceDir', event.target.value)}
            />
            <button
              type="button"
              className="rounded-xl border border-gray-200 px-3 text-xs text-ink-100 hover:border-primary-400/40"
              disabled={!currentDir}
              onClick={() => onConfigChange('sourceDir', currentDir)}
            >
              当前
            </button>
          </div>
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">整理目的地目录（媒体库根目录）</span>
          <div className="flex gap-2">
            <input
              className="input-base w-full"
              placeholder="例如 F:\\media 或 /media"
              value={config.targetDir}
              onChange={(event) => onConfigChange('targetDir', event.target.value)}
            />
            <button
              type="button"
              className="rounded-xl border border-gray-200 px-3 text-xs text-ink-100 hover:border-primary-400/40"
              disabled={!currentDir}
              onClick={() => onConfigChange('targetDir', currentDir)}
            >
              当前
            </button>
          </div>
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">默认整理方式</span>
          <select
            className="input-base w-full"
            value={config.transferMode}
            onChange={(event) => onConfigChange('transferMode', event.target.value)}
          >
            <option value="hardlink">硬链接</option>
            <option value="move">移动（关闭保种才会移动）</option>
            <option value="copy">复制</option>
            <option value="symlink">软链接</option>
          </select>
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">检查间隔（秒）</span>
          <input
            type="number"
            min={60}
            className="input-base w-full"
            value={config.intervalSeconds}
            onChange={(event) => onConfigChange('intervalSeconds', event.target.value)}
          />
        </label>
      </div>

      {moveKeepsSeeding && (
        <div className="rounded-xl border border-orange-300 bg-orange-50 px-3 py-2 text-xs text-orange-700">
          当前同时选择了“移动”和“保种”。为避免 qB 做种源文件被删除，后端会实际使用硬链接；Docker / NAS
          多挂载或不同子卷下可能报 invalid cross-device link。需要真正移动时请关闭“保种”，需要保种但硬链接失败时请选择“复制”。
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <BooleanSetting config={config} settingKey="enabled" label="整理源目录定时自动整理" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="afterDownload" label="qB 下载完成后自动整理" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="downloadSmartClassify" label="下载器智能分类" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="smartClassify" label="智能分类到子库" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="autoAddLibrary" label="自动注册目的地媒体库" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="keepSeeding" label="保种" onConfigChange={onConfigChange} />
      </div>
    </>
  )
}

export function AutoOrganizeNamingTab({ config, onConfigChange }: AutoOrganizeTabProps) {
  return (
    <div className="grid gap-3">
      <TextSetting config={config} settingKey="movieFormat" label="电影命名格式" className="input-base w-full font-mono text-xs" onConfigChange={onConfigChange} />
      <TextSetting config={config} settingKey="tvFormat" label="剧集命名格式" className="input-base w-full font-mono text-xs" onConfigChange={onConfigChange} />
      <TextSetting config={config} settingKey="animeFormat" label="动漫命名格式" className="input-base w-full font-mono text-xs" onConfigChange={onConfigChange} />
      <p className="text-xs text-sand-500">
        可用占位符：{'{title}'} {'{year}'} {'{season}'} {'{season:02}'} {'{episode}'} {'{episode:02}'} {'{category}'}。扩展名会自动补齐。
      </p>
    </div>
  )
}

export function AutoOrganizeScrapeTab({ config, onConfigChange }: AutoOrganizeTabProps) {
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <BooleanSetting config={config} settingKey="scrapeAfter" label="整理后自动刮削" onConfigChange={onConfigChange} />
        <BooleanSetting config={config} settingKey="scrapeAutoOnScan" label="扫描后自动刮削" onConfigChange={onConfigChange} />
      </div>
      <div className="grid gap-3 lg:grid-cols-[1fr_160px_160px_160px]">
        <TextSetting
          config={config}
          settingKey="scrapeProviders"
          label="刮削源优先级"
          placeholder="tmdb,douban,bangumi,thetvdb,fanart"
          onConfigChange={onConfigChange}
        />
        <TextSetting config={config} settingKey="scrapeLanguage" label="首选语言" onConfigChange={onConfigChange} />
        <NumberSetting config={config} settingKey="scrapeDelayMinMs" label="最小间隔 ms" onConfigChange={onConfigChange} />
        <NumberSetting config={config} settingKey="scrapeDelayMaxMs" label="最大间隔 ms" onConfigChange={onConfigChange} />
      </div>
    </>
  )
}

function BooleanSetting({
  config,
  settingKey,
  label,
  onConfigChange,
}: AutoOrganizeTabProps & {
  settingKey: keyof AutoOrganizeConfig
  label: string
}) {
  return (
    <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-ink-100">
      <input
        type="checkbox"
        checked={settingOn(config[settingKey])}
        onChange={(event) => onConfigChange(settingKey, event.target.checked ? 'true' : 'false')}
      />
      {label}
    </label>
  )
}

function TextSetting({
  config,
  settingKey,
  label,
  className = 'input-base w-full',
  placeholder,
  onConfigChange,
}: AutoOrganizeTabProps & {
  settingKey: keyof AutoOrganizeConfig
  label: string
  className?: string
  placeholder?: string
}) {
  return (
    <label className="space-y-1">
      <span className="text-xs text-ink-50">{label}</span>
      <input
        className={className}
        placeholder={placeholder}
        value={config[settingKey]}
        onChange={(event) => onConfigChange(settingKey, event.target.value)}
      />
    </label>
  )
}

function NumberSetting({
  config,
  settingKey,
  label,
  onConfigChange,
}: AutoOrganizeTabProps & {
  settingKey: keyof AutoOrganizeConfig
  label: string
}) {
  return (
    <label className="space-y-1">
      <span className="text-xs text-ink-50">{label}</span>
      <input
        type="number"
        min={0}
        className="input-base w-full"
        value={config[settingKey]}
        onChange={(event) => onConfigChange(settingKey, event.target.value)}
      />
    </label>
  )
}
