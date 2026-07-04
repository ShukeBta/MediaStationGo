import { FormEvent, useState } from 'react'
import toast from 'react-hot-toast'
import { EyeOff, KeyRound, Loader2, Save } from 'lucide-react'

import { authAPI } from '../api/auth'
import { profileAPI } from '../api/profile'
import { requestPassword } from '../components/requestPassword'
import { useAuthStore } from '../stores/auth'

export function ProfilePage() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)

  const [username, setUsername] = useState(user?.username ?? '')
  const [nickname, setNickname] = useState(user?.nickname ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [avatar, setAvatar] = useState(user?.avatar_url ?? '')
  const [hideAdult, setHideAdult] = useState(Boolean(user?.hide_adult))
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [savingProfile, setSavingProfile] = useState(false)
  const [savingPassword, setSavingPassword] = useState(false)

  const onProfile = async (e: FormEvent) => {
    e.preventDefault()
    if (savingProfile) return
    setSavingProfile(true)
    try {
      let password: string | undefined
      const hideAdultChanged = hideAdult !== Boolean(user?.hide_adult)
      const usernameChanged = username.trim() !== (user?.username ?? '')
      if (hideAdultChanged || usernameChanged) {
        const input = await requestPassword({
          title: usernameChanged ? '修改用户名' : hideAdult ? '隐藏成人目录' : '取消隐藏成人目录',
          message: usernameChanged
            ? '修改用户名后需要使用新用户名登录，请输入当前账号密码确认。'
            : '此设置会同步影响 Web 与 Emby/Jellyfin/Infuse 等第三方客户端，请输入当前账号密码确认。',
          confirmText: '保存设置',
        })
        if (!input) return
        password = input
      }
      const patch: Parameters<typeof profileAPI.update>[0] = {
        username,
        nickname,
        email,
        avatar_url: avatar,
        password,
      }
      if (hideAdultChanged) {
        patch.hide_adult = hideAdult
      }
      const u = await profileAPI.update(patch)
      setUser(u)
      toast.success('资料已更新')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSavingProfile(false)
    }
  }

  const onPwd = async (e: FormEvent) => {
    e.preventDefault()
    if (savingPassword) return
    setSavingPassword(true)
    try {
      await authAPI.changePassword(oldPwd, newPwd)
      toast.success('密码已更新')
      setOldPwd('')
      setNewPwd('')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '密码更新失败'
      toast.error(msg)
    } finally {
      setSavingPassword(false)
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">个人资料</h1>

      <form onSubmit={onProfile} className="glass-panel space-y-4">
        <h2 className="font-display text-lg font-semibold text-ink-600">基本信息</h2>
        <Field label="用户名">
          <input
            required
            className="input-base"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />
        </Field>
        <Field label="昵称">
          <input
            className="input-base"
            value={nickname}
            onChange={(e) => setNickname(e.target.value)}
          />
        </Field>
        <Field label="角色">
          <input className="input-base" value={user?.role ?? ''} disabled />
        </Field>
        <Field label="电子邮箱">
          <input
            type="email"
            className="input-base"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </Field>
        <Field label="头像 URL">
          <input
            className="input-base"
            value={avatar}
            onChange={(e) => setAvatar(e.target.value)}
          />
        </Field>
        <label className="flex items-start justify-between gap-4 rounded-2xl border border-gray-200 bg-white/70 p-4">
          <span>
            <span className="flex items-center gap-2 font-medium text-ink-600">
              <EyeOff size={16} /> 隐藏成人目录
            </span>
            <span className="mt-1 block text-sm leading-6 text-ink-50">
              开启后当前账号在网页、外部播放器链接以及 Emby/Jellyfin/Infuse 等第三方客户端中都不会显示成人媒体库和 NSFW 条目。
            </span>
          </span>
          <input
            type="checkbox"
            className="mt-1 h-5 w-5 accent-brand-500"
            checked={hideAdult}
            onChange={(e) => setHideAdult(e.target.checked)}
          />
        </label>
        <button type="submit" disabled={savingProfile} className="neon-button">
          {savingProfile ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          保存
        </button>
      </form>

      <form onSubmit={onPwd} className="glass-panel space-y-4">
        <h2 className="font-display text-lg font-semibold text-ink-600">修改密码</h2>
        <Field label="当前密码">
          <input
            required
            type="password"
            className="input-base"
            value={oldPwd}
            onChange={(e) => setOldPwd(e.target.value)}
            autoComplete="current-password"
          />
        </Field>
        <Field label="新密码">
          <input
            required
            type="password"
            className="input-base"
            minLength={6}
            value={newPwd}
            onChange={(e) => setNewPwd(e.target.value)}
            autoComplete="new-password"
          />
        </Field>
        <button type="submit" disabled={savingPassword} className="neon-button">
          {savingPassword ? <Loader2 size={16} className="animate-spin" /> : <KeyRound size={16} />}
          更新密码
        </button>
      </form>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-ink-100">{label}</span>
      {children}
    </label>
  )
}
