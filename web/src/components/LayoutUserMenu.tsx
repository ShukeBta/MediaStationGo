import { useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { ChevronDown, Loader2, LogOut, RotateCw, Settings, UserCog } from 'lucide-react'
import clsx from 'clsx'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import type { PlayProfile } from '../types'

type LayoutUser = {
  username?: string
  role?: string
}

type LayoutUserMenuProps = {
  user: LayoutUser | null | undefined
  isOpen: boolean
  profiles: PlayProfile[]
  activeProfileId: string | null
  activeProfile: PlayProfile | null
  onToggle: () => void
  onClose: () => void
  onUseDefaultProfile: () => void
  onSwitchProfile: (profile: PlayProfile) => void
  onLogout: () => void
}

export function LayoutUserMenu({
  user,
  isOpen,
  profiles,
  activeProfileId,
  activeProfile,
  onToggle,
  onClose,
  onUseDefaultProfile,
  onSwitchProfile,
  onLogout,
}: LayoutUserMenuProps) {
  const location = useLocation()
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const lastLocationRef = useRef(`${location.pathname}${location.search}`)
  const [updating, setUpdating] = useState(false)

  useEffect(() => {
    if (!isOpen) return undefined

    const handlePointerDown = (event: PointerEvent) => {
      const root = rootRef.current
      const target = event.target
      if (!root || !(target instanceof Node) || root.contains(target)) return
      onClose()
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { onClose(); triggerRef.current?.focus() }
    }

    document.addEventListener('pointerdown', handlePointerDown, true)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [isOpen, onClose])

  useEffect(() => {
    const nextLocation = `${location.pathname}${location.search}`
    if (lastLocationRef.current === nextLocation) return
    lastLocationRef.current = nextLocation
    if (isOpen) onClose()
  }, [isOpen, location.pathname, location.search, onClose])

  const applySystemUpdate = async () => {
    if (updating) return
    setUpdating(true)
    try {
      const status = await adminAPI.systemUpdateApply()
      toast.success(status.message || '系统更新任务已启动')
      onClose()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '启动系统更新失败'
      toast.error(msg)
    } finally {
      setUpdating(false)
    }
  }

  return (
    <div ref={rootRef} className="relative" data-testid="layout-user-menu">
      <button
        ref={triggerRef}
        onClick={onToggle}
        onKeyDown={(event) => {
          if (event.key !== 'ArrowDown') return
          event.preventDefault()
          if (!isOpen) onToggle()
          window.requestAnimationFrame(() => rootRef.current?.querySelector<HTMLElement>('[role="menuitem"], [role="menuitemradio"]')?.focus())
        }}
        aria-label={`账户菜单：${user?.username ?? '用户'}`}
        aria-expanded={isOpen}
        aria-haspopup="menu"
        className="shell-profile-trigger"
      >
        <div className="shell-profile-avatar">
          {user?.username?.slice(0, 2).toUpperCase() || 'US'}
        </div>
        <div className="shell-profile-copy">
          <p className="truncate text-xs font-semibold leading-none text-[var(--app-text)]">{user?.username}</p>
          <p className="mt-0.5 text-[9px] font-bold uppercase leading-none tracking-wider text-[var(--app-muted)]">
            {activeProfile ? activeProfile.name : user?.role === 'admin' ? '管理员' : '观影账户'}
          </p>
        </div>
        <ChevronDown size={14} className="text-[var(--app-muted)]" />
      </button>

      <AnimatePresence>
        {isOpen && (
          <motion.div
            initial={{ opacity: 0, y: 10, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 10, scale: 0.95 }}
            transition={{ duration: 0.15 }}
            role="menu"
            aria-label="账户与观影配置"
            onKeyDown={(event) => {
              if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
              const items = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('a[href], button:not([disabled])'))
              if (items.length === 0) return
              event.preventDefault()
              const current = items.indexOf(document.activeElement as HTMLElement)
              const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
                : event.key === 'ArrowDown' ? (current + 1) % items.length : (current - 1 + items.length) % items.length
              items[next].focus()
            }}
            className="shell-profile-menu"
          >
            {user?.role === 'admin' && (
              <UserMenuLink to="/admin" icon={<Settings size={16} />} label="管理主控制台" onClick={onClose} />
            )}
            {user?.role === 'admin' && (
              <button
                type="button"
                role="menuitem"
                onClick={applySystemUpdate}
                disabled={updating}
                className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-sm text-[var(--app-subtle)] transition-colors hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] disabled:cursor-not-allowed disabled:opacity-60"
              >
                {updating ? <Loader2 size={16} className="animate-spin" /> : <RotateCw size={16} />}
                <span>一键更新系统</span>
              </button>
            )}
            <div className="my-1.5 border-t border-[var(--app-border)]" />
            <div className="px-3 py-2">
              <p className="mb-2 text-[10px] font-bold uppercase tracking-wider text-[var(--app-muted)]">
                观影配置
              </p>
              <div className="space-y-1">
                <button
                  role="menuitemradio"
                  aria-checked={!activeProfileId}
                  onClick={onUseDefaultProfile}
                  className={profileButtonClass(!activeProfileId)}
                >
                  <span>账号默认</span>
                  <span>{!activeProfileId ? '使用中' : ''}</span>
                </button>
                {profiles.map((profile) => (
                  <button
                    role="menuitemradio"
                    aria-checked={activeProfileId === profile.id}
                    key={profile.id}
                    onClick={() => onSwitchProfile(profile)}
                    className={profileButtonClass(activeProfileId === profile.id)}
                  >
                    <span className="truncate">{profile.name}</span>
                    <span className="ml-2 shrink-0">{profile.allow_adult ? '成人' : '安全'}</span>
                  </button>
                ))}
              </div>
            </div>
            <UserMenuLink to="/play-profiles" icon={<UserCog size={16} />} label="管理观影配置" onClick={onClose} />
            <div className="my-1.5 border-t border-[var(--app-border)]" />
            <button
              role="menuitem"
              onClick={onLogout}
              className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-sm text-red-500 transition-colors hover:bg-[var(--app-danger-soft)]"
            >
              <LogOut size={16} />
              <span>安全登出系统</span>
            </button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function UserMenuLink({
  to,
  icon,
  label,
  onClick,
}: {
  to: string
  icon: React.ReactNode
  label: string
  onClick: () => void
}) {
  return (
    <Link
      role="menuitem"
      to={to}
      onClick={onClick}
      className="flex items-center gap-3 rounded-xl px-3 py-2 text-sm text-[var(--app-subtle)] transition-colors hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
    >
      {icon}
      <span>{label}</span>
    </Link>
  )
}

function profileButtonClass(active: boolean): string {
  return clsx(
    'flex w-full items-center justify-between rounded-xl px-2.5 py-2 text-left text-xs transition-colors',
    active
      ? 'bg-[var(--app-active-bg)] text-[var(--app-active-text)]'
      : 'text-[var(--app-subtle)] hover:bg-[var(--app-hover)]',
  )
}
