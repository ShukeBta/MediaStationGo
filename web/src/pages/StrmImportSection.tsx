import type { FormEvent } from 'react'
import { Plus } from 'lucide-react'

import type { Library } from '../types'

type StrmImportSectionProps = {
  libraries: Library[]
  libraryID: string
  title: string
  url: string
  importing: boolean
  onImport: (event: FormEvent) => void
  setLibraryID: (value: string) => void
  setTitle: (value: string) => void
  setURL: (value: string) => void
}

export function StrmImportSection({
  libraries,
  libraryID,
  title,
  url,
  importing,
  onImport,
  setLibraryID,
  setTitle,
  setURL,
}: StrmImportSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <h2 className="font-display text-lg font-semibold text-ink-600">导入 STRM 条目</h2>
      <form onSubmit={onImport} className="grid gap-3 md:grid-cols-4">
        <select
          required
          className="input-base"
          value={libraryID}
          onChange={(e) => setLibraryID(e.target.value)}
        >
          <option value="" disabled>
            选择媒体库
          </option>
          {libraries.map((library) => (
            <option key={library.id} value={library.id}>
              {library.name} ({library.type})
            </option>
          ))}
        </select>
        <input
          required
          className="input-base"
          placeholder="标题"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
        />
        <input
          required
          className="input-base md:col-span-2"
          placeholder="https://example.com/movie.mp4"
          value={url}
          onChange={(e) => setURL(e.target.value)}
        />
        <button type="submit" disabled={importing} className="neon-button md:col-span-4">
          <Plus size={16} /> {importing ? '导入中…' : '导入'}
        </button>
      </form>
      <p className="text-xs text-sand-500">
        导入后会创建一条 container=strm 的媒体记录,播放时会 302 重定向到该 URL。
      </p>
    </section>
  )
}
