export interface LibraryRoot {
  id: string
  library_id: string
  name?: string
  path: string
  enabled: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

export interface Library {
  id: string
  name: string
  path: string
  type: string
  enabled: boolean
  roots?: LibraryRoot[]
  created_at: string
  updated_at: string
}

export interface ScanResult {
  library_id: string
  visited: number
  added: number
  updated?: number
  probed: number
  local_metadata?: number
  removed?: number
  skipped?: number
  discovered?: number
  queued?: boolean
  cloud?: boolean
  message?: string
  estimate_message?: string
}

export interface Setting {
  key: string
  value: string
  updated_at: string
}

export interface AccessLog {
  id: string
  user_id: string
  action: string
  target: string
  ip: string
  detail: string
  created_at: string
}
