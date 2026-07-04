import { api } from './client'
import type { AccessLog, Setting, User } from '../types'

export interface SystemUpdateStatus {
  image: string
  current_version?: string
  watchtower_image?: string
  update_mode?: string
  compose_dir?: string
  compose_file?: string
  compose_command?: string
  container_id?: string
  container_name?: string
  current_image_id?: string
  local_digest?: string
  remote_digest?: string
  docker_available: boolean
  can_apply: boolean
  update_available?: boolean
  running: boolean
  task_id?: string
  message?: string
  details?: string
  checked_at?: string
  started_at?: string
}

export const adminAPI = {
  listUsers: () => api.get<User[]>('/admin/users').then((r) => r.data),

  createUser: (payload: { username: string; password: string }) =>
    api.post<User>('/admin/users', payload).then((r) => r.data),

  updateUser: (id: string, payload: { username: string }) =>
    api.patch<User>(`/admin/users/${id}`, payload).then((r) => r.data),

  resetUserPassword: (id: string, password: string) =>
    api.patch(`/admin/users/${id}/password`, { password }).then((r) => r.data),

  setUserStatus: (id: string, isActive: boolean) =>
    api.patch<User>(`/admin/users/${id}/status`, { is_active: isActive }).then((r) => r.data),

  deleteUser: (id: string) => api.delete(`/admin/users/${id}`).then((r) => r.data),

  listSettings: () => api.get<Setting[]>('/admin/settings').then((r) => r.data),

  updateSetting: (key: string, value: string) =>
    api.put('/admin/settings', { key, value }).then((r) => r.data),

  recentLogs: () => api.get<AccessLog[]>('/admin/logs').then((r) => r.data),

  systemUpdateStatus: () => api.get<SystemUpdateStatus>('/admin/system/update').then((r) => r.data),

  systemUpdateCheck: () => api.post<SystemUpdateStatus>('/admin/system/update/check').then((r) => r.data),

  systemUpdateApply: () => api.post<SystemUpdateStatus>('/admin/system/update/apply').then((r) => r.data),
}
