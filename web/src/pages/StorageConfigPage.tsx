import { useState } from 'react'
import { Cloud } from 'lucide-react'

import {
  type StorageType,
} from '../api/storage_config'
import {
  STORAGE_TABS,
  TYPE_LABEL,
} from './storageConfigModel'
import { StorageForm } from './StorageForm'

// StorageConfigPage manages the OpenList / Alist / WebDAV / CloudDrive2 / 115 adapters used by
// the import / playback / STRM subsystems. Mirrors the Vue UI's
// `admin/storage/*` tabs in a tabbed React surface.
export function StorageConfigPage() {
  const [active, setActive] = useState<StorageType>('openlist')
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-400/10 text-blue-300">
          <Cloud size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-[var(--app-text)]">外部存储</h1>
          <p className="text-sm text-[var(--app-muted)]">
            配置 OpenList / Alist / WebDAV / CloudDrive2 / 115 后端，支持本地转存、网盘挂载和 302/反代播放
          </p>
        </div>
      </div>

      <div className="flex gap-2 border-b border-[var(--app-border)]">
        {STORAGE_TABS.map((t) => (
          <button
            key={t}
            onClick={() => setActive(t)}
            className={
              'border-b-2 px-4 py-2 text-sm ' +
              (active === t
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-[var(--app-muted)] hover:text-[var(--app-text)]')
            }
          >
            {TYPE_LABEL[t] ?? t}
          </button>
        ))}
      </div>

      <StorageForm key={active} type={active} />
    </div>
  )
}
