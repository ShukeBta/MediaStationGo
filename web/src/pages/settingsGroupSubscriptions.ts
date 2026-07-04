import type { SettingGroup } from './settingsGroupTypes'

export const subscriptionSettingsGroup: SettingGroup = {
  key: 'subscriptions',
  label: '订阅任务',
  description: '控制 RSS / 站点搜索订阅的唯一后台轮询频率。手动“立即执行”不受这里限制。',
  items: [
    {
      key: 'subscription.interval_seconds',
      label: '订阅自动同步间隔秒数',
      type: 'number',
      hint: '默认 10800 秒，最小 10800 秒。后台自动刷新固定保守节奏，避免馒头等站点 API 超限。',
      defaultValue: '10800',
    },
  ],
}
