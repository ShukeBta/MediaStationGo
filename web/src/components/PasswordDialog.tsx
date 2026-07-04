import { FormEvent, useState } from 'react'
import { KeyRound } from 'lucide-react'

export type PasswordOptions = {
  title?: string
  message?: string
  confirmText?: string
}

export function PasswordDialog({
  options,
  onClose,
}: {
  options: PasswordOptions
  onClose: (value: string | null) => void
}) {
  const [password, setPassword] = useState('')

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (!password) return
    onClose(password)
  }

  return (
    <div
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm"
      onClick={() => onClose(null)}
    >
      <form
        role="dialog"
        aria-modal="true"
        onSubmit={onSubmit}
        className="w-full max-w-sm overflow-hidden rounded-3xl border border-white/70 bg-white shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex gap-4 p-5">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-primary-400/10 text-brand-500">
            <KeyRound size={22} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-display text-lg font-bold text-ink-600">
              {options.title || '需要密码确认'}
            </h3>
            <p className="mt-2 text-sm leading-6 text-ink-50">
              {options.message || '请输入当前账号密码以继续。'}
            </p>
            <input
              autoFocus
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              className="mt-4 w-full rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-ink-600 outline-none transition focus:border-brand-500 focus:bg-white focus:ring-4 focus:ring-brand-100/40"
              placeholder="当前账号密码"
              autoComplete="current-password"
            />
          </div>
        </div>
        <div className="flex justify-end gap-2 border-t border-gray-100 bg-gray-50/80 px-5 py-4">
          <button
            type="button"
            onClick={() => onClose(null)}
            className="rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-semibold text-ink-100 hover:bg-gray-50"
          >
            取消
          </button>
          <button
            type="submit"
            disabled={!password}
            className="rounded-xl bg-brand-500 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {options.confirmText || '确认'}
          </button>
        </div>
      </form>
    </div>
  )
}
