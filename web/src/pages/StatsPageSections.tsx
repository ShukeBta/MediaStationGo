import type { ReactNode } from 'react'
import { Activity, Cpu, Database, Film, HardDrive, Radio, Users } from 'lucide-react'

import type { Hardware, StatsSnapshot } from '../types'

function fmtBytes(n: number): string {
  if (!n || n <= 0) return '—'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(2)} ${u[i]}`
}

function fmtHours(seconds: number): string {
  if (!seconds || seconds <= 0) return '—'
  const h = Math.floor(seconds / 3600)
  return `${h.toLocaleString()} h`
}

export function StatsHeader({
  generatedAt,
  lastMonitorAt,
  monitorError,
}: {
  generatedAt: string
  lastMonitorAt: string
  monitorError: string
}) {
  return (
    <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1 className="font-display text-3xl font-bold text-ink-600">运行状态</h1>
        <p className="text-sm text-ink-50">
          聚合快照:{new Date(generatedAt).toLocaleString()} · 实时监控每 2 秒刷新
        </p>
      </div>
      <div className="glass-panel inline-flex items-center gap-3 !px-4 !py-3">
        <span className="relative flex h-3 w-3">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-70" />
          <span className="relative inline-flex h-3 w-3 rounded-full bg-emerald-500" />
        </span>
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-emerald-600">
            {monitorError ? '监控重试中' : '实时监控中'}
          </p>
          <p className="text-xs text-ink-50">
            最近刷新:{lastMonitorAt ? new Date(lastMonitorAt).toLocaleTimeString() : '—'}
          </p>
        </div>
      </div>
    </header>
  )
}

export function StatsTiles({
  snap,
  live,
  memPct,
  diskPct,
}: {
  snap: StatsSnapshot
  live: Hardware
  memPct: number
  diskPct: number
}) {
  return (
    <section className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
      <Tile icon={<Database size={20} />} label="媒体库" value={snap.libraries.toLocaleString()} />
      <Tile icon={<Film size={20} />} label="媒体总数" value={snap.media_count.toLocaleString()} />
      <Tile icon={<Users size={20} />} label="用户" value={snap.users_count.toLocaleString()} />
      <Tile icon={<HardDrive size={20} />} label="入库容量" value={fmtBytes(snap.total_size_bytes)} />
      <Tile icon={<Activity size={20} />} label="累计时长" value={fmtHours(snap.total_seconds)} />
      <Tile icon={<Cpu size={20} />} label="CPU 占用" value={`${live.cpu_percent.toFixed(1)}%`} meter={live.cpu_percent} />
      <Tile icon={<Cpu size={20} />} label="内存占用" value={`${memPct.toFixed(1)}%`} />
      <Tile icon={<HardDrive size={20} />} label="数据盘占用" value={`${diskPct.toFixed(1)}%`} />
    </section>
  )
}

export function SystemMonitorSection({
  live,
  memPct,
  diskPct,
  monitorError,
}: {
  live: Hardware
  memPct: number
  diskPct: number
  monitorError: string
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-display text-xl font-semibold text-ink-600">系统实时监控</h2>
        <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-700">
          <Radio size={12} />
          Live
        </span>
      </div>
      <div className="glass-panel grid gap-4 text-sm lg:grid-cols-[1fr_1.4fr]">
        <div className="grid gap-2">
          <Row label="Go 运行时" value={live.go_version} />
          <Row label="Goroutines" value={live.goroutines.toLocaleString()} />
          <Row
            label="内存"
            value={`${fmtBytes(live.memory_used)} / ${fmtBytes(live.memory_total)}`}
          />
          <Row
            label="数据盘"
            value={`${fmtBytes(live.disk_used)} / ${fmtBytes(live.disk_total)}`}
          />
        </div>
        <div className="grid gap-3">
          <Meter label="CPU" value={live.cpu_percent} />
          <Meter label="内存" value={memPct} />
          <Meter label="数据盘" value={diskPct} />
          {monitorError && <p className="text-xs text-red-500">实时监控错误:{monitorError}</p>}
        </div>
      </div>
    </section>
  )
}

export function AggregateStatsSection({ generatedAt }: { generatedAt: string }) {
  return (
    <section className="space-y-3">
      <h2 className="font-display text-xl font-semibold text-ink-600">聚合统计</h2>
      <div className="glass-panel grid gap-2 text-sm">
        <Row
          label="统计刷新"
          value={new Date(generatedAt).toLocaleString()}
        />
        <Row label="统计口径" value="媒体库、用户、容量每 30 秒刷新" />
      </div>
    </section>
  )
}

function Tile({
  icon,
  label,
  value,
  meter,
}: {
  icon: ReactNode
  label: string
  value: string
  meter?: number
}) {
  return (
    <div className="glass-panel flex items-center gap-3 !p-4">
      <div className="rounded-xl border border-primary-400/40 bg-primary-400/10 p-2 text-brand-500">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs uppercase tracking-wider text-sand-500">{label}</p>
        <p className="font-display text-lg font-semibold text-ink-600">{value}</p>
        {typeof meter === 'number' && <Bar value={meter} />}
      </div>
    </div>
  )
}

function Meter({ label, value }: { label: string; value: number }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="font-semibold text-ink-600">{label}</span>
        <span className="font-mono text-ink-50">{value.toFixed(1)}%</span>
      </div>
      <Bar value={value} />
    </div>
  )
}

function Bar({ value }: { value: number }) {
  const pct = Math.max(0, Math.min(100, value || 0))
  const color = pct > 85 ? 'bg-red-500' : pct > 65 ? 'bg-amber-500' : 'bg-emerald-500'
  return (
    <div className="h-2 overflow-hidden rounded-full bg-gray-100">
      <div className={`h-full rounded-full transition-all duration-700 ${color}`} style={{ width: `${pct}%` }} />
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between border-b border-gray-200 pb-1 text-sm last:border-0">
      <span className="text-ink-50">{label}</span>
      <span className="font-mono text-ink-600">{value}</span>
    </div>
  )
}
