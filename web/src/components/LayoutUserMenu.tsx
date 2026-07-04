import { useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { ChevronDown, Loader2, LogOut, RotateCw, Settings, User as UserIcon, UserCog } from 'lucide-react'
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
      if (event.key === 'Escape') onClose()
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
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-haspopup="menu"
        className="flex items-center gap-2.5 rounded-full border border-[var(--app-border)] p-1 pr-3 transition-all hover:bg-[var(--app-hover)]"
      >
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-[#111827] to-[#1f2937] font-display text-xs font-bold text-white shadow-sm">
          {user?.username?.slice(0, 2).toUpperCase() || 'US'}
        </div>
        <div className="hidden text-left md:block">
          <p className="text-xs font-bold leading-none text-[var(--app-text)]">{user?.username}</p>
          <p className="mt-0.5 text-[9px] font-bold uppercase leading-none tracking-wider text-[var(--app-muted)]">
            {activeProfile ? `Profile: ${activeProfile.name}` : user?.role}
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
            className="absolute right-0 z-50 mt-3 w-56 origin-top-right rounded-2xl border border-[var(--app-border)] bg-[var(--app-panel)] p-2 shadow-xl"
          >
            <UserMenuLink to="/profile" icon={<UserIcon size={16} />} label="个人基本信息" onClick={onClose} />
            {user?.role === 'admin' && (
              <UserMenuLink to="/admin" icon={<Settings size={16} />} label="管理主控制台" onClick={onClose} />
            )}
            {user?.role === 'admin' && (
              <button
                type="button"
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
                当前观影 Profile
              </p>
              <div className="space-y-1">
                <button
                  onClick={onUseDefaultProfile}
                  className={profileButtonClass(!activeProfileId)}
                >
                  <span>账号默认</span>
                  <span>{!activeProfileId ? '使用中' : ''}</span>
                </button>
                {profiles.map((profile) => (
                  <button
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
            <UserMenuLink to="/play-profiles" icon={<UserCog size={16} />} label="管理观影 Profile" onClick={onClose} />
            <div className="my-1.5 border-t border-[var(--app-border)]" />
            <button
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
