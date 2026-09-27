import type { SettingGroup } from './settingsGroupTypes'

export const adultSettingsGroup: SettingGroup = {
  key: 'adult',
  label: 'Adult / NSFW',
  description: '成人功能总开关与访问保护',
  items: [
    {
      key: 'adult.enabled',
      label: '全站启用成人功能',
      type: 'toggle',
      hint: '关闭后全站用户均无法访问成人内容；开启后仍受管理员设置的用户限制和播放配置限制。成人专用库请将媒体库类型设为 Adult。',
      defaultValue: 'true',
    },
    {
      key: 'adult.require_pin',
      label: '访问需要 PIN',
      type: 'toggle',
    },
    {
      key: 'adult.pin',
      label: 'PIN 码',
      type: 'text',
      hint: '4-8 位数字',
    },
  ],
}
