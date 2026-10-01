import { useState } from 'react'
import { Link } from 'react-router-dom'
import { CloudDownload, Search } from 'lucide-react'
import type { DiscoverItem } from '../api/discover'
import { DiscoverCloudResourceAction } from './DiscoverCloudResourceAction'
import { PTResourceSearchPanel } from './PTResourceSearchPanel'
import { discoverResourceSearchKeyword } from './discoverDetailModalModel'
import { useResourceImportCapability } from '../hooks/useResourceImportCapability'
import { CloudImportAvailability } from './CloudImportAvailability'

export function DiscoverResourceAction(props: {
  item: DiscoverItem
  sidecarRoot?: HTMLElement | null
  subscriptionActionRoot?: HTMLElement | null
  onSidecarOpenChange?: (open: boolean) => void
}) {
  const [mode, setMode] = useState<'pt' | 'cloud'>('pt')
  const cloud = useResourceImportCapability()
  const { item } = props
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <button type="button" className={mode === 'pt' ? 'btn-primary gap-2' : 'btn-outline gap-2'} onClick={() => setMode('pt')}><Search size={15} />PT 搜索 / 订阅</button>
        <button type="button" className={mode === 'cloud' ? 'btn-primary gap-2' : 'btn-outline gap-2'} onClick={() => setMode('cloud')}><CloudDownload size={15} />网盘入库</button>
      </div>
      {mode === 'cloud' ? cloud.state === 'enabled' ? <DiscoverCloudResourceAction {...props} /> : <CloudImportAvailability {...cloud} /> : <>
        {item.in_library && item.media_id && <Link to={`/media/${item.media_id}`} className="inline-block text-sm text-brand-500">已在媒体库中，查看库内作品</Link>}
        <PTResourceSearchPanel key={`${item.source}:${item.provider_id || item.tmdb_id || item.douban_id || item.title}`} initialQuery={discoverResourceSearchKeyword(item)} metadata={{ ...item, season_number: item.season_num }} />
      </>}
    </section>
  )
}
