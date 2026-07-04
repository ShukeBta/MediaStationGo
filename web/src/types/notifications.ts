export interface NotifyChannel {
  id: string
  name: string
  type: 'telegram' | 'wechat' | 'bark' | 'webhook' | 'email'
  enabled: boolean
  events: string[]
  config: Record<string, string>
  created_at: string
  updated_at: string
}

export interface NotifyProviderInfo {
  type: string
  name: string
  description: string
}
