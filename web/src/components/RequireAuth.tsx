import { ReactNode, useEffect } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { useAuthStore } from '../stores/auth'
import { usePermissionStore } from '../stores/permissions'

// Route guard: redirects to /login when no token is present.
export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token)
  const location = useLocation()
  if (!token) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return <>{children}</>
}

// Route guard: only allows users with role === "admin".
export function RequireAdmin({ children }: { children: ReactNode }) {
  const user = useAuthStore((s) => s.user)
  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />
  }
  return <>{children}</>
}

export function RequirePermission({ permission, children }: { permission: string; children: ReactNode }) {
  const user = useAuthStore((s) => s.user)
  const permissions = usePermissionStore((s) => s.permissions)
  const isSuper = usePermissionStore((s) => s.isSuper)
  const loading = usePermissionStore((s) => s.isLoading)
  const error = usePermissionStore((s) => s.error)
  const fetchPermissions = usePermissionStore((s) => s.fetchPermissions)
  const loaded = Object.keys(permissions).length > 0

  useEffect(() => {
    if (user && user.role !== 'admin' && !loaded && !loading && !error) {
      fetchPermissions().catch(() => undefined)
    }
  }, [error, fetchPermissions, loaded, loading, user])

  if (user?.role === 'admin' || isSuper) return <>{children}</>
  if (error) {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-red-200 bg-red-50 p-5 text-sm text-red-700">
        <p>菜单权限加载失败：{error}</p>
        <button type="button" className="mt-3 rounded-lg border border-red-300 px-3 py-2 font-semibold" onClick={() => void fetchPermissions()}>
          重新加载
        </button>
      </div>
    )
  }
  if (!loaded || loading) return <p className="px-6 py-8 text-sand-500">正在加载菜单权限…</p>
  if (permissions[permission] !== true) {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-800">
        <p className="font-semibold">无权访问此页面</p>
        <p className="mt-1">当前账号未获得所需权限，请联系管理员调整。</p>
      </div>
    )
  }
  return <>{children}</>
}
