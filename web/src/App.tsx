import { Component, Suspense, type ErrorInfo, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'

import { appRoutes, type AppRoute } from './appRoutes'
import { Layout } from './components/Layout'
import { RequireAdmin, RequireAuth } from './components/RequireAuth'
import { LoginPage } from './pages/LoginPage'

const Loading = () => <p className="px-6 py-8 text-sand-500">加载中…</p>

class AppErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  state = { hasError: false }

  static getDerivedStateFromError() {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('MediaStationGo UI crashed', error, info)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen bg-gray-50 px-6 py-16 text-gray-900">
          <div className="mx-auto max-w-xl rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
            <p className="text-lg font-bold">页面加载失败</p>
            <p className="mt-2 text-sm text-gray-500">
              当前页面遇到异常，刷新后会重新加载资源和登录状态。
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-3">
              <a
                href="/"
                className="rounded-xl bg-gray-950 px-4 py-2 text-sm font-bold text-white hover:bg-gray-800"
              >
                返回首页
              </a>
              <button
                type="button"
                onClick={() => window.location.reload()}
                className="rounded-xl border border-gray-300 px-4 py-2 text-sm font-bold text-gray-700 hover:bg-gray-100"
              >
                刷新页面
              </button>
            </div>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

function routeElement(route: AppRoute) {
  if (!route.adminOnly) return route.element
  return <RequireAdmin>{route.element}</RequireAdmin>
}

export default function App() {
  return (
    <AppErrorBoundary>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <RequireAuth>
                <Layout />
              </RequireAuth>
            }
          >
            {appRoutes.map((route) => (
              <Route
                key={route.index ? 'index' : route.path}
                index={route.index}
                path={route.path}
                element={routeElement(route)}
              />
            ))}
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </AppErrorBoundary>
  )
}
