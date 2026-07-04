import { Component, type ErrorInfo, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'

/**
 * 路由级错误边界：只包住当前路由内容（Outlet），崩溃时侧边栏 / 顶栏仍然保留，
 * 用户可以点「返回首页」或直接用侧边栏切换到别的页面（切换 pathname 会让父层
 * 的 keyed motion.div 重新挂载本边界，从而自动恢复），避免整页只剩一个刷新按钮
 * 导致「无法返回」。
 */
function RouteErrorFallback() {
  const navigate = useNavigate()
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <p className="text-lg font-bold text-ink-600">页面加载失败</p>
      <p className="mt-2 max-w-md text-sm text-sand-500">
        当前页面遇到异常。你可以返回首页继续使用，或刷新后重试；其他页面不受影响。
      </p>
      <div className="mt-5 flex flex-wrap items-center justify-center gap-3">
        <button
          type="button"
          onClick={() => navigate('/')}
          className="rounded-xl bg-gray-950 px-4 py-2 text-sm font-bold text-white hover:bg-gray-800"
        >
          返回首页
        </button>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="rounded-xl border border-gray-300 px-4 py-2 text-sm font-bold text-ink-600 hover:bg-gray-100"
        >
          刷新页面
        </button>
      </div>
    </div>
  )
}

export class RouteErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  state = { hasError: false }

  static getDerivedStateFromError() {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('MediaStationGo route crashed', error, info)
  }

  render() {
    if (this.state.hasError) return <RouteErrorFallback />
    return this.props.children
  }
}
