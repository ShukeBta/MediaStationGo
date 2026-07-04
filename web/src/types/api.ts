export interface ApiConfig {
  id: string
  provider: string
  api_key?: string
  base_url?: string
  extra?: string
  enabled: boolean
  description?: string
  last_tested_at?: string
  test_result?: string
  updated_at: string
}

export interface ApiProvider {
  id: string
  name: string
  description: string
  has_api_key: boolean
  has_base_url: boolean
}
