import { useId, type ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
import { ChevronDown } from 'lucide-react'
import clsx from 'clsx'

type SidebarGroupProps = {
  id: string; icon: ReactNode; label: string; children: ReactNode
  collapsed?: boolean; open?: boolean; active?: boolean; onToggle: (id: string) => void
}

export function SidebarGroup({ id, label, children, collapsed, open, active, onToggle }: SidebarGroupProps) {
  const contentId = useId()
  const reduceMotion = useReducedMotion()
  if (collapsed) return <div className="shell-nav-section is-collapsed" role="group" aria-label={label}>{children}</div>
  return (
    <div className="shell-nav-section">
      <button type="button" onClick={() => onToggle(id)} aria-expanded={!!open} aria-controls={contentId}
        className={clsx('shell-nav-heading', active && 'has-active-route')}>
        <span>{label}</span><ChevronDown size={12} className={clsx(open && 'is-open')} />
      </button>
      <AnimatePresence initial={false}>
        {open && <motion.div id={contentId} initial={{ height: 0, opacity: 0 }} animate={{ height: 'auto', opacity: 1 }}
          exit={{ height: 0, opacity: 0 }} transition={{ duration: reduceMotion ? 0 : 0.2, ease: 'easeOut' }} className="shell-nav-items">
          {children}
        </motion.div>}
      </AnimatePresence>
    </div>
  )
}

type SidebarLinkProps = { to: string; icon: ReactNode; label: string; end?: boolean; collapsed?: boolean; child?: boolean }

export function SidebarLink({ to, icon, label, end, collapsed }: SidebarLinkProps) {
  return (
    <NavLink to={to} end={end} title={collapsed ? label : undefined} aria-label={collapsed ? label : undefined}
      className={({ isActive }) => clsx('shell-nav-link', isActive && 'is-active', collapsed && 'is-collapsed')}>
      <span className="shell-nav-icon" aria-hidden="true">{icon}</span>
      {!collapsed && <span className="shell-nav-label">{label}</span>}
    </NavLink>
  )
}
