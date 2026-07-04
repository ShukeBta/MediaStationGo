import { FormEvent, useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { licenseAPI, type LicenseStatus } from '../api/license'
import {
  LicenseBindPanel,
  LicenseHeader,
  LicenseInactiveTip,
  LicenseStatusPanel,
} from './LicensePageSections'

export function LicensePage() {
  const [status, setStatus] = useState<LicenseStatus | null>(null)
  const [loadingStatus, setLoadingStatus] = useState(true)
  const [bindKey, setBindKey] = useState('')
  const [binding, setBinding] = useState(false)

  const refreshStatus = useCallback(async () => {
    setLoadingStatus(true)
    try {
      const s = await licenseAPI.status()
      setStatus(s)
    } catch {
      setStatus({ active: false })
    } finally {
      setLoadingStatus(false)
    }
  }, [])

  useEffect(() => {
    refreshStatus()
  }, [refreshStatus])

  const onBind = async (e: FormEvent) => {
    e.preventDefault()
    const key = bindKey.trim()
    if (!key) {
      toast.error('请输入许可证密钥')
      return
    }
    setBinding(true)
    try {
      const activation = await licenseAPI.bind(key)
      toast.success('许可证绑定成功!')
      setBindKey('')
      // Optimistically update status
      setStatus({
        active: true,
        activation,
        max_users: activation.max_users,
        unlimited_users: activation.unlimited_users,
        message: '已激活',
      })
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '绑定失败，请检查密钥是否正确'
      toast.error(msg)
    } finally {
      setBinding(false)
    }
  }

  const onHeartbeat = async () => {
    try {
      await licenseAPI.heartbeat()
      toast.success('心跳上报成功')
    } catch {
      toast.error('心跳上报失败')
    }
  }

  // ── Derive display values ──
  const active = status?.active === true
  const activation = status?.activation
  const isExpired =
    activation?.expires_at != null && new Date(activation.expires_at).getTime() < Date.now()

  return (
    <div className="mx-auto max-w-2xl space-y-8">
      <LicenseHeader />
      <LicenseBindPanel
        bindKey={bindKey}
        binding={binding}
        onBind={onBind}
        onBindKeyChange={setBindKey}
      />
      <LicenseStatusPanel
        status={status}
        loadingStatus={loadingStatus}
        active={active}
        isExpired={isExpired}
        onRefresh={refreshStatus}
        onHeartbeat={onHeartbeat}
      />
      <LicenseInactiveTip active={active} loadingStatus={loadingStatus} />
    </div>
  )
}
