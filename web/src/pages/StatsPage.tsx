import { useEffect, useState } from 'react'

import { statsAPI } from '../api/stats'
import type { Hardware, StatsSnapshot } from '../types'
import {
  AggregateStatsSection,
  StatsHeader,
  StatsTiles,
  SystemMonitorSection,
} from './StatsPageSections'

// StatsPage renders the operator dashboard. Aggregate stats refresh less often,
// while hardware metrics are polled every 2 s for a real-time monitoring feel.
export function StatsPage() {
  const [snap, setSnap] = useState<StatsSnapshot | null>(null)
  const [hardware, setHardware] = useState<Hardware | null>(null)
  const [lastMonitorAt, setLastMonitorAt] = useState<string>('')
  const [monitorError, setMonitorError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    const tick = () =>
      statsAPI.snapshot().then((s) => {
        if (!cancelled) {
          setSnap(s)
          setHardware((current) => current ?? s.hardware)
          setLastMonitorAt((current) => current || s.generated_at)
        }
      })
    tick().finally(() => setLoading(false))
    const id = window.setInterval(tick, 30_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    const tick = () =>
      statsAPI
        .monitor()
        .then((m) => {
          if (!cancelled) {
            setHardware(m)
            setLastMonitorAt(new Date().toISOString())
            setMonitorError('')
          }
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            setMonitorError((err as { message?: string })?.message ?? '实时监控暂不可用')
          }
        })
    tick()
    const id = window.setInterval(tick, 2_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [])

  if (loading) return <p className="text-sand-500">加载中…</p>
  if (!snap) return <p className="text-sand-500">无法获取统计数据</p>

  const live = hardware ?? snap.hardware
  const memPct =
    live.memory_total > 0
      ? (live.memory_used / live.memory_total) * 100
      : 0
  const diskPct =
    live.disk_total > 0
      ? (live.disk_used / live.disk_total) * 100
      : 0

  return (
    <div className="space-y-8">
      <StatsHeader
        generatedAt={snap.generated_at}
        lastMonitorAt={lastMonitorAt}
        monitorError={monitorError}
      />
      <StatsTiles snap={snap} live={live} memPct={memPct} diskPct={diskPct} />
      <SystemMonitorSection
        live={live}
        memPct={memPct}
        diskPct={diskPct}
        monitorError={monitorError}
      />
      <AggregateStatsSection generatedAt={snap.generated_at} />
    </div>
  )
}
