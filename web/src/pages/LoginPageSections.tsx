import { useId } from 'react'
import type { CSSProperties, FormEvent, ReactNode } from 'react'
import { ArrowRight, Eye, EyeOff, Play, LoaderCircle, LockKeyhole, UserRound } from 'lucide-react'

import { AppFooter } from '../components/AppFooter'
import { useLoginShowcase } from './useLoginShowcase'

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
  icon: ReactNode
  autoFocus?: boolean
  trailing?: ReactNode
  onChange: (value: string) => void
}

export function LoginPageShell({ children }: LoginPageShellProps) {
  const { current, posters, count, next } = useLoginShowcase()
  return (
    <main className="cinema-auth">
      <section className="cinema-auth-scene" aria-label="你的私人影院">
        <div className="cinema-auth-scene-shade" />
        <div className="cinema-auth-brand">
          <span className="cinema-auth-mark"><Play size={20} fill="currentColor" strokeWidth={1.5} aria-hidden="true" /></span>
          <span>MediaStation<span className="cinema-auth-brand-go">Go</span></span>
        </div>
        {posters.length > 0 && <div className="cinema-auth-posters" aria-label="片库海报">
          {posters.map((item, index) => {
            const position = index - Math.floor(posters.length / 2)
            return <figure key={`${current?.artwork_url}-${item.artwork_url}`} className="cinema-auth-poster" data-edge={Math.abs(position) > 2} style={{ '--poster-x': `${position * 43}%`, '--poster-y': `${Math.abs(position) * 12}px`, '--poster-rotation': `${position * 7}deg`, '--poster-delay': `${(3 - Math.abs(position)) * 90}ms`, zIndex: 10 - Math.ceil(Math.abs(position)) } as CSSProperties}>
              <img src={item.artwork_url} alt={item.title} fetchPriority={item === current ? 'high' : 'auto'} onError={event => { event.currentTarget.style.visibility = 'hidden' }} />
            </figure>
          })}
        </div>}
        <div className="cinema-auth-scene-copy">
          <span className="cinema-auth-eyebrow"><span /> {current ? `片库珍藏${current.year > 0 ? ` · ${current.year}` : ''}` : '私人片库'}</span>
          <h2 key={`title-${current?.artwork_url || 'empty'}`}>{current ? current.title : <>好故事，<br />值得慢慢看。</>}</h2>
          <p>{current ? current.overview || '已收录在你的私人片库，登录后探索这部作品。' : '收藏热爱，把每一次播放，留给属于自己的时光。'}</p>
          {count > 1 && <button type="button" className="cinema-auth-next" onClick={next}>换一部看看 <ArrowRight size={15} aria-hidden="true" /></button>}
        </div>
        <div className="cinema-auth-scene-bottom" aria-hidden="true">
          <span>LIGHTS DOWN. WORLD ON.</span>
          <span className="cinema-auth-frame"><span /><span /><span /></span>
        </div>
      </section>

      <section className="cinema-auth-access" aria-label="账号登录">
        <div className="cinema-auth-access-top"><span className="cinema-auth-live-dot" /> 你的媒体，尽在这里</div>
        {children}
        <div className="cinema-auth-footer"><AppFooter /></div>
      </section>
    </main>
  )
}

export function LoginCard(props: LoginCardProps) {
  return (
    <div className="cinema-auth-card">
      <header className="cinema-auth-card-heading">
        <p className="cinema-auth-overline">WELCOME BACK</p>
        <h1>欢迎回来<span>。</span></h1>
        <p className="cinema-auth-description">登录，继续你的光影旅程。</p>
      </header>
      <LoginForm {...props} />
      <p className="cinema-auth-private"><LockKeyhole size={13} aria-hidden="true" /> 私人空间 · 专属珍藏</p>
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
  const autoFocusUsername = typeof window !== 'undefined' && window.matchMedia('(min-width: 768px)').matches

  return (
    <form onSubmit={onSubmit} className="cinema-auth-form" aria-busy={loading}>
      <LoginInput
        label="用户名"
        type="text"
        value={username}
        autoComplete="username"
        placeholder="输入你的账号"
        icon={<UserRound size={18} strokeWidth={1.65} />}
        autoFocus={autoFocusUsername}
        onChange={onUsernameChange}
      />
      <LoginInput
        label="密码"
        type={showPassword ? 'text' : 'password'}
        value={password}
        autoComplete="current-password"
        placeholder="输入登录密码"
        icon={<LockKeyhole size={18} strokeWidth={1.65} />}
        trailing={<PasswordVisibilityButton visible={showPassword} onToggle={onTogglePassword} />}
        onChange={onPasswordChange}
      />
      <button type="submit" disabled={loading} className="cinema-auth-submit">
        {loading ? (
          <><span>正在登录</span><LoaderCircle className="cinema-auth-spinner" size={19} aria-hidden="true" /></>
        ) : (
          <><span>进入我的影院</span><ArrowRight size={19} aria-hidden="true" /></>
        )}
      </button>
    </form>
  )
}

function LoginInput({
  label,
  value,
  type,
  autoComplete,
  placeholder,
  icon,
  autoFocus,
  trailing,
  onChange,
}: LoginInputProps) {
  const id = useId()
  return (
    <div className="cinema-auth-field">
      <label htmlFor={id}>{label}</label>
      <div className="cinema-auth-input-wrap">
        <span className="cinema-auth-input-icon" aria-hidden="true">{icon}</span>
        <input
          id={id}
          name={autoComplete === 'username' ? 'username' : 'password'}
          type={type}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          autoComplete={autoComplete}
          autoCapitalize="none"
          spellCheck={false}
          autoFocus={autoFocus}
          required
          placeholder={placeholder}
        />
        {trailing}
      </div>
    </div>
  )
}

function PasswordVisibilityButton({ visible, onToggle }: { visible: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="cinema-auth-password-toggle"
      aria-label={visible ? '隐藏密码' : '显示密码'}
      aria-pressed={visible}
      title={visible ? '隐藏密码' : '显示密码'}
    >
      {visible ? <EyeOff size={17} strokeWidth={1.65} /> : <Eye size={17} strokeWidth={1.65} />}
    </button>
  )
}
