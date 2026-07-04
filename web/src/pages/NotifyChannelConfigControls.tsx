import { Field } from './NotifyChannelFormField'

type ConfigControlProps = {
  config: Record<string, string>
  updateConfig: (key: string, value: string) => void
}

type ConfigInputProps = ConfigControlProps & {
  label: string
  name: string
  placeholder?: string
  required?: boolean
  type?: string
}

export function ConfigInput({
  config,
  updateConfig,
  label,
  name,
  placeholder,
  required,
  type,
}: ConfigInputProps) {
  return (
    <Field label={label}>
      <input
        required={required}
        type={type}
        className="input-base"
        placeholder={placeholder}
        value={config[name] ?? ''}
        onChange={(event) => updateConfig(name, event.target.value)}
      />
    </Field>
  )
}

type ConfigSelectProps = ConfigControlProps & {
  label: string
  name: string
  options: Array<{ value: string; label: string }>
  defaultValue?: string
}

export function ConfigSelect({
  config,
  updateConfig,
  label,
  name,
  options,
  defaultValue = '',
}: ConfigSelectProps) {
  return (
    <Field label={label}>
      <select
        className="input-base"
        value={config[name] ?? defaultValue}
        onChange={(event) => updateConfig(name, event.target.value)}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </Field>
  )
}

type ConfigTextareaProps = ConfigControlProps & {
  label: string
  name: string
  placeholder?: string
  rows: number
}

export function ConfigTextarea({
  config,
  updateConfig,
  label,
  name,
  placeholder,
  rows,
}: ConfigTextareaProps) {
  return (
    <Field label={label}>
      <textarea
        rows={rows}
        className="input-base font-mono text-xs"
        placeholder={placeholder}
        value={config[name] ?? ''}
        onChange={(event) => updateConfig(name, event.target.value)}
      />
    </Field>
  )
}
