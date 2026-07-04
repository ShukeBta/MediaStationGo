import { adultSettingsGroup } from './settingsGroupAccess'
import { cloudUploadSettingsGroup } from './settingsGroupCloud'
import { generalSettingsGroup, licenseSettingsGroup } from './settingsGroupGeneral'
import { recognitionWordsSettingsGroup } from './settingsGroupRecognitionWords'
import { subscriptionSettingsGroup } from './settingsGroupSubscriptions'
import { systemUpdateSettingsGroup } from './settingsGroupSystemUpdate'
import type { SettingGroup } from './settingsGroupTypes'

export type { SettingGroup } from './settingsGroupTypes'

export const GROUPS: SettingGroup[] = [
  generalSettingsGroup,
  licenseSettingsGroup,
  systemUpdateSettingsGroup,
  recognitionWordsSettingsGroup,
  subscriptionSettingsGroup,
  cloudUploadSettingsGroup,
  adultSettingsGroup,
]

export const ALL_KEYS = new Set(GROUPS.flatMap((group) => group.items.map((item) => item.key)))
