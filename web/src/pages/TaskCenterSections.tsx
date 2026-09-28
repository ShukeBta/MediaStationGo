import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { tasksAPI, type BackgroundTask, type TaskDefinition, type TaskLog, type TaskLogDay, type TaskPage, type PendingScrape } from '../api/tasks'
import { libraryAPI } from '../api/library'

const today = () => new Date().toISOString().slice(0, 10)
const monthAgo = () => new Date(Date.now() - 29 * 86400000).toISOString().slice(0, 10)
const field = 'rounded-lg border border-gray-200 bg-white px-2 py-1 text-sm'
const button = `${field} disabled:opacity-40`
const errorText = (e: unknown) => (e as { response?: { data?: { error?: string } } })?.response?.data?.error || (e instanceof Error ? e.message : '任务操作失败')

function Pager({ page, total, size = 25, change }: { page: number; total: number; size?: number; change: (page: number) => void }) {
  return <div className="mt-3 flex items-center gap-3 text-sm"><button className={button} disabled={page <= 1} onClick={() => change(page - 1)}>上一页</button><span>{page} / {Math.max(1, Math.ceil(total / size))} 页，共 {total} 条</span><button className={button} disabled={page * size >= total} onClick={() => change(page + 1)}>下一页</button></div>
}

export function TaskDefinitionsSection() {
  const [items, setItems] = useState<TaskDefinition[]>([])
  const [pending, setPending] = useState<string>('')
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    const refresh = () => tasksAPI.definitions().then(r => { if (active) { setItems(r); setError('') } }).catch(e => { if (active) setError(errorText(e)) })
    void refresh()
    const timer = window.setInterval(() => void refresh(), 5000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])
  async function run(key: string) {
    setPending(key)
    try { await tasksAPI.run(key); toast.success('任务已进入后台执行'); setItems(await tasksAPI.definitions()) } catch (e) { toast.error(errorText(e)) } finally { setPending('') }
  }
  return <section className="glass-panel space-y-3"><h2 className="text-lg font-semibold text-ink-600">任务目录与手动执行</h2><p className="text-sm text-sand-500">定时任务沿用各自的启用、时间窗口与周期设置。运行中的同一任务不会重复启动。</p>{error && <p role="alert" className="text-red-600">{error}</p>}<div className="overflow-x-auto"><table className="w-full min-w-[620px] text-left text-sm"><thead><tr><th>任务</th><th>触发方式 / 周期</th><th>状态</th><th>操作</th></tr></thead><tbody>{items.map(item => <tr key={item.key} className="border-t border-gray-200"><td className="py-2">{item.name}{item.last_error && <p className="max-w-md break-words text-xs text-red-600">{item.last_error}</p>}</td><td>{item.trigger}{item.schedule && ` · ${item.schedule}`}</td><td>{item.running ? '运行中' : '空闲'}</td><td>{item.manual && <button className={button} disabled={item.running || !!pending} onClick={() => void run(item.key)}>{pending === item.key ? '正在触发…' : '执行一次'}</button>}</td></tr>)}</tbody></table></div></section>
}

export function TaskHistorySection() {
  const [from, setFrom] = useState(monthAgo)
  const [to, setTo] = useState(today)
  const [kind, setKind] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [revision, setRevision] = useState(0)
  const [history, setHistory] = useState<TaskPage<BackgroundTask>>()
  const [days, setDays] = useState<TaskLogDay[]>([])
  const [day, setDay] = useState(today)
  const [taskID, setTaskID] = useState('')
  const [logPage, setLogPage] = useState(1)
  const [logs, setLogs] = useState<TaskPage<TaskLog>>()
  const [error, setError] = useState('')
  const [logError, setLogError] = useState('')
  useEffect(() => {
    let active = true
    const filter = { from, to, kind, status, page, page_size: 25 }
    Promise.all([tasksAPI.history(filter), tasksAPI.logDays({ from, to, kind, task_id: taskID })]).then(([h, d]) => { if (active) { setHistory(h); setDays(d); setError('') } }).catch(e => { if (active) setError(errorText(e)) })
    return () => { active = false }
  }, [from, to, kind, status, page, taskID, revision])
  useEffect(() => {
    let active = true
    tasksAPI.logs({ from: day, to: day, kind, task_id: taskID, page: logPage, page_size: 50 }).then(r => { if (active) { setLogs(r); setLogError('') } }).catch(e => { if (active) setLogError(errorText(e)) })
    return () => { active = false }
  }, [day, kind, taskID, logPage, revision])
  function selectTask(task: BackgroundTask) { setTaskID(task.id); setDay(task.updated_at.slice(0, 10)); setLogPage(1) }
  return <section className="glass-panel space-y-4"><h2 className="text-lg font-semibold text-ink-600">执行历史与每日日志</h2><div className="flex flex-wrap items-center gap-2"><label>开始 <input className={field} type="date" value={from} onChange={e => { setFrom(e.target.value); setPage(1) }} /></label><label>结束 <input className={field} type="date" value={to} onChange={e => { setTo(e.target.value); setPage(1) }} /></label><input className={field} aria-label="任务类型" placeholder="任务类型（可留空）" value={kind} onChange={e => { setKind(e.target.value); setPage(1); setLogPage(1) }} /><select className={field} aria-label="任务状态" value={status} onChange={e => { setStatus(e.target.value); setPage(1) }}><option value="">全部状态</option><option value="running">运行中</option><option value="completed">完成</option><option value="failed">失败</option><option value="canceled">取消</option></select><button className={button} onClick={() => setRevision(v => v + 1)}>刷新</button></div><p className="text-xs text-sand-500">日期统一按 UTC 分组。执行历史在重启后保留；日志不保留原始 URL 或凭据。</p>{error && <p role="alert" className="text-red-600">{error}</p>}<div className="overflow-x-auto"><table className="w-full min-w-[600px] text-left text-sm"><thead><tr><th>任务 / 类型</th><th>状态</th><th>开始时间</th><th>结果</th><th /></tr></thead><tbody>{history?.items.map(task => <tr key={task.id} className="border-t border-gray-200"><td className="py-2">{task.name}<p className="text-xs text-sand-500">{task.kind}</p></td><td>{task.status}</td><td>{new Date(task.started_at).toLocaleString()}</td><td className="max-w-md break-words">{task.error || task.message || '—'}</td><td><button className={button} onClick={() => selectTask(task)}>查看日志</button></td></tr>)}</tbody></table></div>{history && <Pager page={page} total={history.total} change={setPage} />}<div className="border-t border-gray-200 pt-4"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">每日执行日志</h3><input className={field} aria-label="日志日期" type="date" value={day} onChange={e => { setDay(e.target.value); setLogPage(1) }} />{taskID && <button className={button} onClick={() => { setTaskID(''); setLogPage(1) }}>清除单任务筛选</button>}</div><div className="my-2 flex flex-wrap gap-2">{days.map(item => <button key={item.day} className={`${button} ${day === item.day ? 'font-bold text-brand-500' : ''}`} onClick={() => { setDay(item.day); setLogPage(1) }}>{item.day} ({item.count})</button>)}</div>{logError && <p role="alert" className="text-red-600">{logError}</p>}<div className="max-h-[440px] overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-3 font-mono text-xs">{logs?.items.length === 0 && <p>该日期暂无日志。</p>}{logs?.items.map(log => <div key={log.id} className={`whitespace-pre-wrap break-words border-b border-gray-200 py-1 ${log.level === 'error' ? 'text-red-600' : ''}`}>{new Date(log.logged_at).toISOString().slice(11, 19)} [{log.kind}] {log.message}</div>)}</div>{logs && <Pager page={logPage} total={logs.total} size={50} change={setLogPage} />}</div></section>
}

export function PendingScrapeSection({ onDeleteSTRM }: { onDeleteSTRM?: (mediaID: string) => void }) {
  const [library, setLibrary] = useState('')
  const [libraries, setLibraries] = useState<{ id: string; name: string }[]>([])
  const [page, setPage] = useState(1)
  const [revision, setRevision] = useState(0)
  const [data, setData] = useState<TaskPage<PendingScrape>>()
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  useEffect(() => { let active = true; libraryAPI.list().then(r => { if (active) setLibraries(r) }).catch(e => { if (active) setError(errorText(e)) }); return () => { active = false } }, [])
  useEffect(() => { let active = true; tasksAPI.pending(library, page).then(r => { if (active) { setData(r); setError('') } }).catch(e => { if (active) setError(errorText(e)) }); return () => { active = false } }, [library, page, revision])
  async function scrape(id: string) { setPending(id); try { await tasksAPI.scrape(id); toast.success('刮削结束'); setRevision(v => v + 1) } catch (e) { toast.error(errorText(e)) } finally { setPending('') } }
  return <section className="glass-panel space-y-3"><h2 className="text-lg font-semibold text-ink-600">待刮削媒体</h2><div className="flex gap-2"><select className={field} aria-label="筛选媒体库" value={library} onChange={e => { setLibrary(e.target.value); setPage(1) }}><option value="">全部媒体库</option>{libraries.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select><button className={button} onClick={() => setRevision(v => v + 1)}>刷新</button></div>{error && <p role="alert" className="text-red-600">{error}</p>}<div className="overflow-x-auto"><table className="w-full min-w-[500px] text-left text-sm"><thead><tr><th>媒体</th><th>状态</th><th>操作</th></tr></thead><tbody>{data?.items.map(item => <tr key={item.id} className="border-t border-gray-200"><td className="py-2"><Link to={`/media/${item.id}`} className="text-brand-500">{item.title}</Link>{item.is_strm && <span className="ml-2 text-xs text-sand-500">STRM</span>}</td><td>{item.scrape_status || 'pending'}</td><td className="space-x-2"><button className={button} disabled={!!pending} onClick={() => void scrape(item.id)}>{pending === item.id ? '刮削中…' : '重试刮削'}</button>{item.is_strm && onDeleteSTRM && <button className={button} onClick={() => onDeleteSTRM(item.id)}>删除目标</button>}</td></tr>)}</tbody></table></div>{data?.items.length === 0 && <p className="text-sm text-sand-500">暂无待刮削媒体。</p>}{data && <Pager page={page} total={data.total} change={setPage} />}</section>
}
