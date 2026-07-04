import { Loader2, LogOut, Save, Send } from 'lucide-react'

type StorageFormActionsProps = {
  showLogout: boolean
  loggingOut: boolean
  testing: boolean
  saving: boolean
  onLogout: () => void
  onTest: () => void
}

export function StorageFormActions({
  showLogout,
  loggingOut,
  testing,
  saving,
  onLogout,
  onTest,
}: StorageFormActionsProps) {
  return (
    <div className="flex justify-end gap-2 pt-2">
      {showLogout && (
        <button
          type="button"
          onClick={onLogout}
          disabled={loggingOut}
          className="rounded-lg border border-red-300/60 px-4 py-2 text-sm text-red-500 hover:bg-[var(--app-danger-soft)] disabled:opacity-50"
        >
          {loggingOut ? <Loader2 size={14} className="inline animate-spin" /> : <LogOut size={14} className="inline" />}
          {' '}退出登录
        </button>
      )}
      <button
        type="button"
        onClick={onTest}
        disabled={testing}
        className="rounded-lg border border-[var(--app-border)] px-4 py-2 text-sm text-[var(--app-subtle)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
      >
        {testing ? <Loader2 size={14} className="inline animate-spin" /> : <Send size={14} className="inline" />}
        {' '}测试
      </button>
      <button type="submit" disabled={saving} className="neon-button">
        {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
        保存
      </button>
    </div>
  )
}
