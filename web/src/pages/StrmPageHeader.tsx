import { Link as LinkIcon } from 'lucide-react'

export function StrmPageHeader() {
  return (
    <div className="flex items-center gap-3">
      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-400/10 text-amber-400">
        <LinkIcon size={20} />
      </div>
      <div>
        <h1 className="font-display text-3xl font-bold text-ink-600">STRM 管理</h1>
        <p className="text-sm text-ink-50">
          将外部 HTTP / WebDAV / Alist 直链以&quot;虚拟文件&quot;形式纳入媒体库
        </p>
      </div>
    </div>
  )
}
