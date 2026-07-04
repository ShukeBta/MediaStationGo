import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { ChevronDown } from 'lucide-react'
import clsx from 'clsx'

type SidebarGroupProps = {
  id: string
  icon: ReactNode
  label: string
  children: ReactNode
  collapsed?: boolean
  open?: boolean
  active?: boolean
  onToggle: (id: string) => void
}

export function SidebarGroup({ id, icon, label, children, collapsed, open, active, onToggle }: SidebarGroupProps) {
  return (
    <div className="space-y-1">
      <button
        type="button"
        onClick={() => onToggle(id)}
        className={clsx(
          'group relative flex w-full items-center gap-3.5 rounded-xl px-4 py-3 text-sm font-bold transition-all duration-300',
          active
            ? 'bg-[var(--app-active-bg)] text-[var(--app-active-text)] shadow-sm'
            : 'text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]',
          collapsed && 'justify-center px-0',
        )}
      >
        <span className={clsx(
          'flex h-5 w-5 shrink-0 items-center justify-center',
          active ? 'text-[var(--app-active-icon)]' : 'text-[var(--app-muted)] group-hover:text-[var(--app-subtle)]',
        )}>
          {icon}
        </span>
        {!collapsed && (
          <>
            <span className="flex-1 truncate text-left">{label}</span>
            <ChevronDown
              size={14}
              className={clsx('transition-transform duration-200', open && 'rotate-180')}
            />
          </>
        )}
        {collapsed && (
          <div className="absolute left-full z-50 ml-3 rounded-xl bg-[var(--app-tooltip-bg)] px-2.5 py-1.5 text-xs font-semibold text-[var(--app-tooltip-text)] opacity-0 shadow-lg transition-opacity group-hover:opacity-100">
            {label}
          </div>
        )}
      </button>
      <AnimatePresence initial={false}>
        {!collapsed && open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="overflow-hidden"
          >
            <div className="space-y-1 pb-1 pl-3">
              {children}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

type SidebarLinkProps = {
  to: string
  icon: ReactNode
  label: string
  end?: boolean
  collapsed?: boolean
  child?: boolean
}

export function SidebarLink({ to, icon, label, end, collapsed, child }: SidebarLinkProps) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        clsx(
          'relative flex items-center gap-3.5 rounded-xl px-4 py-3 text-sm font-semibold transition-all duration-300 group',
          child && 'py-2.5 text-[13px]',
          isActive
            ? 'bg-[var(--app-active-bg)] text-[var(--app-active-text)] shadow-sm'
            : 'text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]',
        )
      }
    >
      {({ isActive }) => (
        <>
          <span className={clsx(
            'flex h-5 w-5 shrink-0 items-center justify-center transition-transform duration-300 group-hover:scale-110',
            isActive ? 'text-[var(--app-active-icon)]' : 'text-[var(--app-muted)] group-hover:text-[var(--app-subtle)]',
          )}>
            {icon}
          </span>
          {!collapsed && (
            <motion.span
              initial={{ opacity: 0, x: -5 }}
              animate={{ opacity: 1, x: 0 }}
              className="truncate whitespace-nowrap"
            >
              {label}
            </motion.span>
          )}
          {collapsed && (
            <div className="pointer-events-none absolute left-full z-50 ml-3 whitespace-nowrap rounded-xl bg-[var(--app-tooltip-bg)] px-2.5 py-1.5 text-xs font-semibold text-[var(--app-tooltip-text)] opacity-0 shadow-lg transition-opacity group-hover:pointer-events-auto group-hover:opacity-100">
              {label}
            </div>
          )}
        </>
      )}
    </NavLink>
  )
}
