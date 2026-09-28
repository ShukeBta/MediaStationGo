import { useState } from 'react'
import toast from 'react-hot-toast'
import { ScanLine } from 'lucide-react'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'

export function MediaProbeBackfillButton({ libraryID }: { libraryID?: string }) {
  const user = useAuthStore((state) => state.user)
  const [busy, setBusy] = useState(false)
  if (user?.role !== 'admin') return null
  const run = async () => {
    setBusy(true)
    try {
      const response = await api.post('/media/probes/missing', null, { params: { library_id: libraryID } })
      toast.success(response.data.already_running ? '已有轨道回填任务运行，请在任务中心查看。' : '已开始补齐媒体轨道，进度可在任务中心查看。')
    } catch {
      toast.error('轨道回填启动失败，请稍后重试。')
    } finally {
      setBusy(false)
    }
  }
  return <button type="button" disabled={busy} onClick={() => void run()} className="btn-outline"><ScanLine size={14} />{busy ? '正在启动…' : '补齐媒体轨道'}</button>
}
