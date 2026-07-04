import { Monitor, Moon, Sun } from 'lucide-react'
import clsx from 'clsx'

import type { ThemeMode } from './useThemeMode'

type LayoutThemeToggleProps = {
  mode: ThemeMode
  onChange: (mode: ThemeMode) => void
}

const options: Array<{
  mode: ThemeMode
  label: string
  icon: typeof Sun
}> = [
  { mode: 'light', label: '白天模式', icon: Sun },
  { mode: 'dark', label: '夜晚模式', icon: Moon },
  { mode: 'system', label: '跟随系统', icon: Monitor },
]

export function LayoutThemeToggle({ mode, onChange }: LayoutThemeToggleProps) {
  return (
    <div className="flex items-center rounded-full border border-[var(--app-border)] bg-[var(--app-control-bg)] p-1 shadow-sm">
      {options.map((option) => {
        const Icon = option.icon
        const active = mode === option.mode
        return (
          <button
            key={option.mode}
            type="button"
            title={option.label}
            aria-label={option.label}
            aria-pressed={active}
            className={clsx(
              'inline-flex h-8 w-8 items-center justify-center rounded-full transition-all duration-200',
              active
                ? 'bg-[var(--app-command-bg)] text-[var(--app-command-text)] shadow-sm'
                : 'text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]',
            )}
            onClick={() => onChange(option.mode)}
          >
            <Icon size={15} />
          </button>
        )
      })}
    </div>
  )
}
