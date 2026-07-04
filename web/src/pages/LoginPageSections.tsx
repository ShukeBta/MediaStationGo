import type { FormEvent, ReactNode } from 'react'
import { motion } from 'framer-motion'
import { ArrowRight, Eye, EyeOff, Lock, User } from 'lucide-react'

import { AppFooter } from '../components/AppFooter'

type LoginPageShellProps = {
  children: ReactNode
}

type LoginCardProps = {
  username: string
  password: string
  showPassword: boolean
  loading: boolean
  onUsernameChange: (value: string) => void
  onPasswordChange: (value: string) => void
  onTogglePassword: () => void
  onSubmit: (event: FormEvent) => void
}

type LoginInputProps = {
  label: string
  value: string
  type: string
  autoComplete: string
  placeholder: string
  delay: number
  icon: ReactNode
  autoFocus?: boolean
  trailing?: ReactNode
  onChange: (value: string) => void
}

export function LoginPageShell({ children }: LoginPageShellProps) {
  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center overflow-hidden bg-gray-50/50 px-4">
      <LoginBackground />
      {children}
      <LoginFooter />
    </div>
  )
}

function LoginBackground() {
  return (
    <div className="pointer-events-none absolute inset-0 z-0">
      <div className="absolute -right-20 -top-20 h-[600px] w-[600px] rounded-full bg-brand-100/30 blur-[130px]" />
      <div className="absolute -bottom-40 -left-20 h-[500px] w-[500px] rounded-full bg-sage-100/30 blur-[110px]" />
      <div className="absolute inset-0 bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] [background-size:20px_20px] opacity-40" />
    </div>
  )
}

export function LoginCard(props: LoginCardProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 30 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}
      className="relative z-10 w-full max-w-[420px] px-2"
    >
      <div className="rounded-3xl border border-gray-200/90 bg-white p-8 shadow-[0_25px_60px_rgba(17,24,39,0.04),0_1px_3px_rgba(17,24,39,0.01)] sm:p-10">
        <LoginBrandHeader />
        <LoginForm {...props} />
      </div>
    </motion.div>
  )
}

function LoginBrandHeader() {
  return (
    <div className="flex flex-col items-center pb-8 text-center">
      <motion.img
        initial={{ scale: 0.8, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        transition={{ delay: 0.15, type: 'spring', stiffness: 200 }}
        src="/brand/mediastationgo-logo.svg"
        alt="MediaStationGo"
        className="mb-4 h-14 w-14 rounded-2xl object-contain shadow-sm"
      />

      <motion.h1
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.25 }}
        className="font-display text-2xl font-extrabold tracking-tight text-gray-900"
      >
        MediaStationGo
      </motion.h1>

      <motion.p
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.35 }}
        className="mt-2 text-xs font-bold uppercase tracking-widest text-[#c9954a]"
      >
        一站式媒体管理中心
      </motion.p>
    </div>
  )
}

function LoginForm({
  username,
  password,
  showPassword,
  loading,
  onUsernameChange,
  onPasswordChange,
  onTogglePassword,
  onSubmit,
}: LoginCardProps) {
  return (
    <form onSubmit={onSubmit} className="space-y-5">
      <LoginInput
        label="用户名"
        type="text"
        value={username}
        autoComplete="username"
        placeholder="请输入您的账号"
        delay={0.4}
        icon={<User size={16} />}
        autoFocus
        onChange={onUsernameChange}
      />
      <LoginInput
        label="安全密码"
        type={showPassword ? 'text' : 'password'}
        value={password}
        autoComplete="current-password"
        placeholder="••••••••"
        delay={0.45}
        icon={<Lock size={16} />}
        trailing={<PasswordVisibilityButton visible={showPassword} onToggle={onTogglePassword} />}
        onChange={onPasswordChange}
      />
      <LoginSubmitButton loading={loading} />
    </form>
  )
}

function LoginInput({
  label,
  value,
  type,
  autoComplete,
  placeholder,
  delay,
  icon,
  autoFocus,
  trailing,
  onChange,
}: LoginInputProps) {
  return (
    <motion.div initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ delay }}>
      <label className="mb-2 block text-xs font-bold uppercase tracking-widest text-gray-500">{label}</label>
      <div className="relative">
        <span className="absolute left-4 top-1/2 -translate-y-1/2 text-gray-500">{icon}</span>
        <input
          type={type}
          className="w-full rounded-xl border border-gray-200 bg-gray-50/50 px-11 py-3.5 pr-12 text-sm text-gray-900 placeholder-gray-500 outline-none transition-all duration-300 focus:border-brand-500 focus:bg-white focus:ring-4 focus:ring-brand-100/50"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          autoComplete={autoComplete}
          autoFocus={autoFocus}
          required
          placeholder={placeholder}
        />
        {trailing}
      </div>
    </motion.div>
  )
}

function PasswordVisibilityButton({ visible, onToggle }: { visible: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="absolute right-4 top-1/2 -translate-y-1/2 text-gray-500 transition-colors hover:text-gray-900"
      tabIndex={-1}
    >
      {visible ? <EyeOff size={16} /> : <Eye size={16} />}
    </button>
  )
}

function LoginSubmitButton({ loading }: { loading: boolean }) {
  return (
    <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.5 }} className="pt-2">
      <button
        type="submit"
        disabled={loading}
        className="flex w-full items-center justify-center gap-2 rounded-xl bg-[#111827] py-3.5 text-sm font-bold text-white transition-all duration-300 hover:-translate-y-0.5 hover:bg-[#1f2937] active:translate-y-0 active:scale-[0.98] disabled:opacity-50"
      >
        {loading ? <LoginSpinnerLabel /> : <LoginReadyLabel />}
      </button>
    </motion.div>
  )
}

function LoginSpinnerLabel() {
  return (
    <span className="flex items-center gap-2">
      <svg className="h-4 w-4 animate-spin text-white" viewBox="0 0 24 24" fill="none">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      正在进入系统舱...
    </span>
  )
}

function LoginReadyLabel() {
  return (
    <span className="flex items-center gap-2">
      立即开启观影之旅 <ArrowRight size={16} />
    </span>
  )
}

function LoginFooter() {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ delay: 0.6 }}
      className="relative z-10 mt-10 text-gray-500 transition-colors hover:text-gray-600"
    >
      <AppFooter />
    </motion.div>
  )
}
