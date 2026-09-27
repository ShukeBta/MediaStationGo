// 权限 API 模块
import { api } from './client'
import type { UserPermission } from '../types'

type PermissionPayload = {
  permissions: Record<string, boolean>
  role: string
  tier: string
  is_super: boolean
}

type ApiEnvelope<T> = {
  code?: number
  message?: string
  data?: T
}

function unwrapPermissionsPayload(raw: unknown): PermissionPayload {
  const payload = (
    raw &&
    typeof raw === 'object' &&
    'data' in raw &&
    (raw as ApiEnvelope<PermissionPayload>).data
      ? (raw as ApiEnvelope<PermissionPayload>).data
      : raw
  ) as Partial<PermissionPayload> | null

  return {
    permissions: payload?.permissions ?? {},
    role: payload?.role ?? '',
    tier: payload?.tier ?? 'free',
    is_super: payload?.is_super ?? false,
  }
}

// 获取用户权限
export async function getUserPermissions(userId: string): Promise<UserPermission> {
  const resp = await api.get<UserPermission>(`/admin/users/${userId}/permissions`)
  return resp.data as unknown as UserPermission
}

// 更新用户权限
export async function updateUserPermissions(
  userId: string,
  permissions: Record<string, boolean>
): Promise<void> {
  await api.put(`/admin/users/${userId}/permissions`, permissions)
}

// 重置用户权限为默认值
export async function resetUserPermissions(userId: string): Promise<void> {
  await api.post(`/admin/users/${userId}/permissions/reset`)
}

// 获取当前用户权限
export async function getMyPermissions(): Promise<{
  permissions: Record<string, boolean>
  role: string
  tier: string
  is_super: boolean
}> {
  const resp = await api.get('/auth/permissions')
  return unwrapPermissionsPayload(resp.data)
}
