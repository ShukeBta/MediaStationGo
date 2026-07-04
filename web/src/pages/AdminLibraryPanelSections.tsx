import { FormEvent } from 'react'
import { Plus, Trash2 } from 'lucide-react'

import type { RootDraft } from './adminLibraryPanelModel'

type CreateFormProps = {
  name: string
  type: string
  roots: RootDraft[]
  onNameChange: (value: string) => void
  onTypeChange: (value: string) => void
  onRootChange: (index: number, patch: Partial<RootDraft>) => void
  onAddRoot: () => void
  onRemoveRoot: (index: number) => void
  onSubmit: (e: FormEvent) => void
}

export function AdminLibraryCreateForm({
  name,
  type,
  roots,
  onNameChange,
  onTypeChange,
  onRootChange,
  onAddRoot,
  onRemoveRoot,
  onSubmit,
}: CreateFormProps) {
  return (
    <form onSubmit={onSubmit} className="glass-panel grid gap-3 md:grid-cols-4">
      <input
        required
        className="input-base"
        placeholder="名称"
        value={name}
        onChange={(e) => onNameChange(e.target.value)}
      />
      <select className="input-base" value={type} onChange={(e) => onTypeChange(e.target.value)}>
        <option value="movie">电影</option>
        <option value="tv">电视剧</option>
        <option value="variety">综艺</option>
        <option value="anime">动漫</option>
        <option value="music">音乐</option>
      </select>
      <div className="md:col-span-4 space-y-2">
        {roots.map((root, index) => (
          <CreateRootRow
            key={index}
            root={root}
            index={index}
            canRemove={roots.length > 1}
            onChange={onRootChange}
            onRemove={onRemoveRoot}
          />
        ))}
        <button type="button" className="inline-flex items-center gap-2 rounded-lg border px-3 py-2 text-sm" onClick={onAddRoot}>
          <Plus size={16} /> 添加路径
        </button>
      </div>
      <p className="md:col-span-4 -mt-2 text-xs text-sand-500">
        名称和类型与现有媒体库一致时，会自动把这里填写的路径追加到该媒体库；Docker
        部署请优先填写容器内路径，例如 /media/电影、/media/电视剧/国产剧。
      </p>
      <button type="submit" className="neon-button md:col-span-4">
        新建 / 追加路径
      </button>
    </form>
  )
}

type CreateRootRowProps = {
  root: RootDraft
  index: number
  canRemove: boolean
  onChange: (index: number, patch: Partial<RootDraft>) => void
  onRemove: (index: number) => void
}

function CreateRootRow({ root, index, canRemove, onChange, onRemove }: CreateRootRowProps) {
  return (
    <div className="grid gap-2 rounded-xl border border-gray-200/80 bg-white/60 p-2 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]">
      <input
        className="input-base"
        placeholder="路径名称"
        value={root.name ?? ''}
        onChange={(e) => onChange(index, { name: e.target.value })}
      />
      <input
        required={index === 0}
        className="input-base"
        placeholder="容器路径，如 /media/电视剧/国产剧"
        value={root.path}
        onChange={(e) => onChange(index, { path: e.target.value })}
      />
      <button
        type="button"
        className="rounded-lg border border-red-400/40 px-3 text-red-400 hover:bg-red-400/10 disabled:opacity-40"
        disabled={!canRemove}
        onClick={() => onRemove(index)}
        title="删除路径"
      >
        <Trash2 size={16} />
      </button>
    </div>
  )
}
