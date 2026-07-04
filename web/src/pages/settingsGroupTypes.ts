import type { SettingDef } from './SettingsRow'

export interface SettingGroup {
  key: string
  label: string
  description?: string
  items: SettingDef[]
}
