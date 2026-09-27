import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'
import { licenseAPI, type LicenseStatus } from '../api/license'
import { getUserPermissions, resetUserPermissions, updateUserPermissions } from '../api/permission'
import type { Library, User, UserPermission } from '../types'
import { confirmAction } from '../components/confirmAction'
import { requestPassword } from '../components/requestPassword'
import { AdminUsersForm } from './AdminUsersForm'
import { AdminUserLibraryAccessModal } from './AdminUserLibraryAccessModal'
import { AdminUserMenuPermissionModal } from './AdminUserMenuPermissionModal'
import { AdminUserHistoryModal } from './AdminUserHistoryModal'
import { AdminUsersTable } from './AdminUsersTable'

export function AdminUsersPanel() {
  const [users, setUsers] = useState<User[]>([])
  const [libraries, setLibraries] = useState<Library[]>([])
  const [licenseStatus, setLicenseStatus] = useState<LicenseStatus | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [editingID, setEditingID] = useState<string | null>(null)
  const [editingUsername, setEditingUsername] = useState('')
  const [resettingPasswordID, setResettingPasswordID] = useState<string | null>(null)
  const [libraryAccessUser, setLibraryAccessUser] = useState<User | null>(null)
  const [loadingLibraryAccessID, setLoadingLibraryAccessID] = useState<string | null>(null)
  const [savingLibraryAccess, setSavingLibraryAccess] = useState(false)
  const [permissionUser, setPermissionUser] = useState<User | null>(null)
  const [userPermission, setUserPermission] = useState<UserPermission | null>(null)
  const [loadingPermissionID, setLoadingPermissionID] = useState<string | null>(null)
  const [savingPermission, setSavingPermission] = useState(false)
  const [resettingPermission, setResettingPermission] = useState(false)
  const [updatingAdultContentID, setUpdatingAdultContentID] = useState<string | null>(null)
  const [historyUser, setHistoryUser] = useState<User | null>(null)
  const refresh = async () => {
    const [nextUsers, nextLicense] = await Promise.all([
      adminAPI.listUsers(),
      licenseAPI.status().catch(() => null),
    ])
    setUsers(nextUsers)
    setLicenseStatus(nextLicense)
  }
  useEffect(() => {
    refresh().catch(() => undefined)
    const timer = window.setInterval(() => refresh().catch(() => undefined), 10000)
    return () => window.clearInterval(timer)
  }, [])

  const unlimitedUsers =
    licenseStatus?.active === true &&
    (licenseStatus.unlimited_users === true || licenseStatus.max_users == null)
  const maxUsers = unlimitedUsers ? null : (licenseStatus?.max_users ?? 20)
  const userLimitReached = maxUsers != null && users.length >= maxUsers
  const userLimitLabel = unlimitedUsers ? '不限制' : String(maxUsers)

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await adminAPI.createUser({ username, password })
      toast.success('用户已添加，默认仅允许浏览与播放媒体')
      setUsername('')
      setPassword('')
      await refresh()
    } catch (err: unknown) {
      const msg =
        userCreateErrorMessage(err) ??
        '添加用户失败'
      toast.error(msg)
    }
  }

  const startEdit = (u: User) => {
    setEditingID(u.id)
    setEditingUsername(u.username)
  }

  const saveEdit = async (id: string) => {
    try {
      await adminAPI.updateUser(id, { username: editingUsername })
      toast.success('用户名已更新')
      setEditingID(null)
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '更新失败'
      toast.error(msg)
    }
  }

  const resetPassword = async (u: User) => {
    if (resettingPasswordID) return
    const nextPassword = await requestPassword({
      title: `重置 ${u.username} 的密码`,
      message: '请输入新的临时密码，至少 6 位。保存后该用户可立即使用新密码登录 Web、Bot 与第三方客户端。',
      confirmText: '重置密码',
    })
    if (!nextPassword) return
    if (nextPassword.length < 6) {
      toast.error('新密码至少 6 位')
      return
    }
    setResettingPasswordID(u.id)
    try {
      await adminAPI.resetUserPassword(u.id, nextPassword)
      toast.success('密码已重置')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '重置密码失败'
      toast.error(msg)
    } finally {
      setResettingPasswordID(null)
    }
  }

  const toggleStatus = async (u: User) => {
    const next = !u.is_active
    if (!next && u.is_protected) {
      toast.error('受保护管理员不可禁用')
      return
    }
    if (
      !next &&
      !(await confirmAction({
        title: '禁用用户',
        message: `禁用「${u.username}」后，Web 与第三方客户端已有登录也会失效。`,
        confirmText: '禁用',
      }))
    ) {
      return
    }
    try {
      await adminAPI.setUserStatus(u.id, next)
      toast.success(next ? '用户已解禁' : '用户已禁用')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '操作失败'
      toast.error(msg)
    }
  }

  const deleteUser = async (u: User) => {
    if (u.is_protected) return
    if (!(await confirmAction({ title: '删除用户', message: `确定删除「${u.username}」?`, confirmText: '删除' }))) return
    await adminAPI.deleteUser(u.id)
    toast.success('已删除')
    await refresh()
  }

  const saveLibraryAccess = async (allowedLibraryIDs: string[]) => {
    if (!libraryAccessUser || savingLibraryAccess) return
    setSavingLibraryAccess(true)
    try {
      await adminAPI.setUserLibraries(libraryAccessUser.id, allowedLibraryIDs)
      toast.success(allowedLibraryIDs.length === 0 ? '已清空媒体库访问权限' : '媒体库访问范围已更新')
      setLibraryAccessUser(null)
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '更新媒体库访问范围失败'
      toast.error(msg)
    } finally {
      setSavingLibraryAccess(false)
    }
  }

  const toggleAdultContentBlocked = async (u: User) => {
    if (u.role === 'admin' || updatingAdultContentID) return
    const blocked = !u.adult_content_blocked
    const confirmed = await confirmAction({
      title: blocked ? '屏蔽成人内容' : '解除成人内容屏蔽',
      message: blocked
        ? `屏蔽「${u.username}」后，首页、最近入库、搜索、媒体详情、播放及第三方客户端都会立即过滤成人作品。`
        : `解除「${u.username}」的管理员强制屏蔽后，仍会继续遵循全局开关和播放配置。`,
      confirmText: blocked ? '确认屏蔽' : '解除屏蔽',
    })
    if (!confirmed) return
    setUpdatingAdultContentID(u.id)
    try {
      const updated = await adminAPI.setUserAdultContentBlocked(u.id, blocked)
      setUsers((current) => current.map((item) => (item.id === updated.id ? updated : item)))
      toast.success(blocked ? '已严格屏蔽该用户的成人内容' : '已解除管理员强制屏蔽')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '更新成人内容限制失败'
      toast.error(msg)
    } finally {
      setUpdatingAdultContentID(null)
    }
  }

  const openLibraryAccess = async (user: User) => {
    if (user.role === 'admin' || loadingLibraryAccessID) return
    setLoadingLibraryAccessID(user.id)
    try {
      const nextLibraries = await libraryAPI.list({ includeHidden: true })
      setLibraries(nextLibraries)
      setLibraryAccessUser(user)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '加载媒体库失败'
      toast.error(msg)
    } finally {
      setLoadingLibraryAccessID(null)
    }
  }

  const openMenuPermission = async (user: User) => {
    if (user.role === 'admin' || loadingPermissionID) return
    setLoadingPermissionID(user.id)
    try {
      const permission = await getUserPermissions(user.id)
      setUserPermission(permission)
      setPermissionUser(user)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '加载菜单权限失败'
      toast.error(msg)
    } finally {
      setLoadingPermissionID(null)
    }
  }

  const saveMenuPermission = async (permissions: Record<string, boolean>) => {
    if (!permissionUser || savingPermission) return
    setSavingPermission(true)
    try {
      await updateUserPermissions(permissionUser.id, permissions)
      toast.success('菜单权限已更新')
      setPermissionUser(null)
      setUserPermission(null)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '更新菜单权限失败'
      toast.error(msg)
    } finally {
      setSavingPermission(false)
    }
  }

  const resetMenuPermission = async () => {
    if (!permissionUser || resettingPermission) return
    setResettingPermission(true)
    try {
      await resetUserPermissions(permissionUser.id)
      const permission = await getUserPermissions(permissionUser.id)
      setUserPermission(permission)
      toast.success('已恢复默认菜单权限')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '重置菜单权限失败'
      toast.error(msg)
    } finally {
      setResettingPermission(false)
    }
  }

  return (
    <div className="space-y-6">
      <AdminUsersForm
        usersCount={users.length}
        userLimitLabel={userLimitLabel}
        username={username}
        password={password}
        userLimitReached={userLimitReached}
        onUsernameChange={setUsername}
        onPasswordChange={setPassword}
        onSubmit={handleCreate}
      />

      <AdminUsersTable
        users={users}
        editingID={editingID}
        editingUsername={editingUsername}
        resettingPasswordID={resettingPasswordID}
        onEditingUsernameChange={setEditingUsername}
        onSaveEdit={saveEdit}
        onCancelEdit={() => setEditingID(null)}
        onStartEdit={startEdit}
        onResetPassword={resetPassword}
        onToggleStatus={toggleStatus}
        onDeleteUser={deleteUser}
        loadingLibraryAccessID={loadingLibraryAccessID}
        onManageLibraries={openLibraryAccess}
        loadingPermissionID={loadingPermissionID}
        onManagePermissions={openMenuPermission}
        updatingAdultContentID={updatingAdultContentID}
        onToggleAdultContentBlocked={toggleAdultContentBlocked}
        onViewHistory={setHistoryUser}
      />

      {libraryAccessUser && (
        <AdminUserLibraryAccessModal
          key={libraryAccessUser.id}
          user={libraryAccessUser}
          libraries={libraries}
          saving={savingLibraryAccess}
          onClose={() => setLibraryAccessUser(null)}
          onSave={saveLibraryAccess}
        />
      )}

      {permissionUser && userPermission && (
        <AdminUserMenuPermissionModal
          key={`${permissionUser.id}-${userPermission.updated_at}`}
          user={permissionUser}
          permission={userPermission}
          saving={savingPermission}
          resetting={resettingPermission}
          onClose={() => {
            setPermissionUser(null)
            setUserPermission(null)
          }}
          onSave={saveMenuPermission}
          onReset={resetMenuPermission}
        />
      )}

      {historyUser && (
        <AdminUserHistoryModal
          key={historyUser.id}
          user={historyUser}
          onClose={() => setHistoryUser(null)}
        />
      )}
    </div>
  )
}

function userCreateErrorMessage(err: unknown): string | undefined {
  const data = (err as { response?: { data?: { error?: string; max_users?: number } } })?.response?.data
  if (!data?.error) return undefined
  if (data.error === 'user limit reached' && data.max_users != null) {
    return `用户数量已达到授权上限：${data.max_users} 人`
  }
  return data.error
}
