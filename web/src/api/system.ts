import { api } from './client'

// systemAPI wraps the read-only /system/* metadata endpoints used by the
// React shell (footer, admin dashboard, scheduler page).
export const systemAPI = {
  info: () =>
    api
      .get<{
        name: string
        version: string
        go: string
        os: string
        arch: string
        data_dir: string
        cache_dir: string
        direct_play_only?: boolean
      }>('/system/info')
      .then((r) => r.data),

  status: () =>
    api
      .get<{
        uptime_seconds: number
        goroutines: number
        cpu_percent?: number
        memory_used?: number
        memory_total?: number
        disk_used?: number
        disk_total?: number
      }>('/system/status')
      .then((r) => r.data),

  scheduler: () =>
    api
      .get<{ jobs: { name: string; cron: string; next_run?: string; last_run?: string }[] }>(
        '/system/scheduler',
      )
      .then((r) => r.data),
}
