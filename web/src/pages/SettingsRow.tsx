import type { Library } from '../types'

export interface SettingDef {
  key: string
  label: string
  type: 'text' | 'select' | 'toggle' | 'number' | 'textarea' | 'library-multiselect'
  hint?: string
  defaultValue?: string
  options?: { value: string; label: string }[]
  placeholder?: string
}

type SettingsRowProps = {
  def: SettingDef
  value: string
  onChange: (value: string) => void
  libraries?: Library[]
}

export function SettingRow({ def, value, onChange, libraries = [] }: SettingsRowProps) {
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
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {def.type === 'number' && (
          <input
            type="number"
            className="input-base"
            value={value}
            placeholder={def.placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {def.type === 'textarea' && (
          <textarea
            rows={3}
            className="input-base font-mono text-xs"
            value={value}
            placeholder={def.placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {def.type === 'select' && (
          <select className="input-base" value={value} onChange={(event) => onChange(event.target.value)}>
            <option value="">(未设置)</option>
            {def.options?.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
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
              onChange={(event) => onChange(event.target.checked ? 'true' : 'false')}
            />
            <span className="text-sm text-ink-100">{toggleOn ? '已启用' : '已关闭'}</span>
          </label>
        )}
        {def.type === 'library-multiselect' && (
          <LibraryMultiSelect
            libraries={libraries}
            selectedLibraryIDs={selectedLibraryIDs}
            onToggle={toggleLibrary}
          />
        )}
      </div>
    </div>
  )
}

function LibraryMultiSelect({
  libraries,
  selectedLibraryIDs,
  onToggle,
}: {
  libraries: Library[]
  selectedLibraryIDs: string[]
  onToggle: (id: string, checked: boolean) => void
}) {
  return (
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
            onChange={(event) => onToggle(lib.id, event.target.checked)}
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
