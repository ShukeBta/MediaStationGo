import type { STRMOutputPreset } from '../api/strm'

type StrmOutputDirPickerProps = {
  className?: string
  inputClassName?: string
  presets: STRMOutputPreset[]
  placeholder: string
  value: string
  required?: boolean
  onChange: (value: string) => void
}

export function StrmOutputDirPicker({
  className = '',
  inputClassName = '',
  presets,
  placeholder,
  value,
  required = false,
  onChange,
}: StrmOutputDirPickerProps) {
  const normalizedValue = value.trim()
  const selectedPreset = presets.some((preset) => preset.path === normalizedValue) ? normalizedValue : ''

  return (
    <div className={`grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(10rem,16rem)] ${className}`}>
      <input
        required={required}
        className={`input-base ${inputClassName}`}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <select
        className="input-base"
        value={selectedPreset}
        onChange={(e) => {
          if (e.target.value) onChange(e.target.value)
        }}
      >
        <option value="">选择输出目录</option>
        {presets.map((preset) => (
          <option key={`${preset.kind}:${preset.path}`} value={preset.path}>
            {preset.kind === 'library' ? `${preset.label} - ${preset.path}` : preset.label}
          </option>
        ))}
      </select>
    </div>
  )
}
