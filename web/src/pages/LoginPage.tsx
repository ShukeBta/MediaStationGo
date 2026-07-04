import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'

import { authAPI } from '../api/auth'
import { useAuthStore } from '../stores/auth'
import { LoginCard, LoginPageShell } from './LoginPageSections'

export function LoginPage() {
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true)
    try {
      const data = await authAPI.login(username, password)
      setSession(data.tokens.access_token, data.tokens.refresh_token, data.user)
      toast.success(`欢迎回来, ${data.user.username}`)
      navigate('/')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '登录失败'
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <LoginPageShell>
      <LoginCard
        username={username}
        password={password}
        showPassword={showPassword}
        loading={loading}
        onUsernameChange={setUsername}
        onPasswordChange={setPassword}
        onTogglePassword={() => setShowPassword((visible) => !visible)}
        onSubmit={handleSubmit}
      />
    </LoginPageShell>
  )
}
