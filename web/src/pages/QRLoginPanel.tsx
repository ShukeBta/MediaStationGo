import { useEffect, useState } from 'react'
import { Loader2, QrCode } from 'lucide-react'
import toast from 'react-hot-toast'

import { cloudAPI, type QRSession, type StorageType } from '../api/storage_config'

interface QRLoginPanelProps {
  type: StorageType
  onCookie: (cookie: string) => void
}

export function QRLoginPanel({ type, onCookie }: QRLoginPanelProps) {
  const [sess, setSess] = useState<QRSession | null>(null)
  const [state, setState] = useState<string>('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!sess) return
    let alive = true
    const timer = setInterval(async () => {
      try {
        const st = await cloudAPI.qrPoll(type, sess)
        if (!alive) return
        setState(st.state)
        if (st.state === 'confirmed' && st.cookie) {
          onCookie(st.cookie)
          toast.success('扫码登录成功,Cookie 已填入,请点击保存')
          setSess(null)
        } else if (st.state === 'expired') {
          toast.error('二维码已过期,请重新获取')
          setSess(null)
        }
      } catch {
        /* keep polling */
      }
    }, 2000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [onCookie, sess, type])

  const start = async () => {
    setBusy(true)
    try {
      const nextSession = await cloudAPI.qrStart(type)
      setSess(nextSession)
      setState('waiting')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '获取二维码失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-lg border border-gray-200 p-3">
      <button
        type="button"
        onClick={start}
        disabled={busy}
        className="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:bg-gray-50"
      >
        {busy ? <Loader2 size={14} className="animate-spin" /> : <QrCode size={14} />}
        使用 115 App 扫码登录
      </button>
      {sess && (
        <div className="mt-3 flex items-center gap-3">
          <img src={sess.qr_image_url} alt="115 QR" className="h-40 w-40 rounded bg-white p-1" />
          <span className="text-sm text-ink-50">
            {state === 'scanned' ? '已扫描,请在手机上确认登录…' : '请使用 115 手机 App 扫描二维码…'}
          </span>
        </div>
      )}
    </div>
  )
}
