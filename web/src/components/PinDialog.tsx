import { FormEvent, useState } from 'react'
import { LockKeyhole } from 'lucide-react'

export type PinOptions = {
  title?: string
  message?: string
  profileName: string
}

export function PinDialog({
  options,
  onClose,
}: {
  options: PinOptions
  onClose: (value: string | null) => void
}) {
  const [pin, setPin] = useState('')

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const trimmed = pin.trim()
    if (!trimmed) return
    onClose(trimmed)
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
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-amber-50 text-amber-500">
            <LockKeyhole size={22} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-display text-lg font-bold text-ink-600">
              {options.title || '需要 PIN 验证'}
            </h3>
            <p className="mt-2 text-sm leading-6 text-ink-50">
              {options.message || `切换到「${options.profileName}」前请输入 PIN。`}
            </p>
            <input
              autoFocus
              type="password"
              inputMode="numeric"
              minLength={4}
              maxLength={8}
              value={pin}
              onChange={(event) => setPin(event.target.value)}
              className="mt-4 w-full rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-center text-lg font-bold tracking-[0.35em] text-ink-600 outline-none transition focus:border-brand-500 focus:bg-white focus:ring-4 focus:ring-brand-100/40"
              placeholder="••••"
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
            disabled={!pin.trim()}
            className="rounded-xl bg-brand-500 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            验证并切换
          </button>
        </div>
      </form>
    </div>
  )
}
