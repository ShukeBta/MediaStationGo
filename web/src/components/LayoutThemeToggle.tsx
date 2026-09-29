import { Monitor, Moon, Sun } from 'lucide-react'
import clsx from 'clsx'
import { motion } from 'framer-motion'

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
    <div className="shell-theme-toggle" role="group" aria-label="界面主题">
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
              'shell-theme-option', active && 'is-active',
            )}
            onClick={() => onChange(option.mode)}
          >
            {active && <motion.span className="shell-theme-indicator" layoutId="shell-theme-indicator" transition={{ type: 'spring', stiffness: 420, damping: 30 }} />}
            <Icon size={15} />
          </button>
        )
      })}
    </div>
  )
}
