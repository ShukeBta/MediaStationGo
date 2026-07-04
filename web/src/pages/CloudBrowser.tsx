import type { StorageType } from '../api/storage_config'
import { CloudBrowserToolbar } from './CloudBrowserToolbar'
import { CloudEntryList } from './CloudEntryList'
import { CloudMountList } from './CloudMountList'
import { CloudScanPanel } from './CloudScanPanel'
import { useCloudBrowser } from './useCloudBrowser'

// Lists cloud directories and imports a file as a 302-backed media.
export function CloudBrowser({ type, enabled = true }: { type: StorageType; enabled?: boolean }) {
  const browser = useCloudBrowser(type, enabled)

  return (
    <div className="mt-2 rounded-lg border border-[var(--app-border)] bg-[var(--app-panel)] p-3">
      <CloudScanPanel
        scanBusy={browser.scanBusy}
        cancelBusy={browser.cancelBusy}
        scanStatuses={browser.scanStatuses}
        onScanAll={browser.scanAllCloudLibraries}
        onCancelScans={browser.cancelCloudScans}
      />
      <CloudMountList mounts={browser.mounts} onRemove={browser.removeMount} />
      <CloudBrowserToolbar
        stack={browser.stack}
        mountMediaType={browser.mountMediaType}
        mounting={browser.mounting}
        batchMounting={browser.batchMounting}
        loading={browser.loading}
        hasDirectories={browser.hasDirectories}
        onGoTo={browser.goTo}
        onGoUp={browser.goUp}
        onCreateFolder={browser.createFolder}
        onMediaTypeChange={browser.setMountMediaType}
        onMountCurrent={browser.mountCurrent}
        onMountVisibleDirectories={browser.mountVisibleDirectories}
      />
      <CloudEntryList
        loading={browser.loading}
        error={browser.error}
        items={browser.items}
        onEnter={browser.enter}
        onImport={browser.doImport}
        onRename={browser.renameFolder}
      />
    </div>
  )
}
