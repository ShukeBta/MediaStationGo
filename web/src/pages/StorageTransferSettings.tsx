type TransferMode = 'copy' | 'move'

type StorageTransferSettingsProps = {
  transferEnabled: boolean
  transferMode: TransferMode
  onTransferEnabledChange: (enabled: boolean) => void
  onTransferModeChange: (mode: TransferMode) => void
}

export function StorageTransferSettings({
  transferEnabled,
  transferMode,
  onTransferEnabledChange,
  onTransferModeChange,
}: StorageTransferSettingsProps) {
  return (
    <div className="rounded-xl border border-blue-300/30 bg-blue-500/10 p-3">
      <div className="mb-2 text-sm font-semibold text-[var(--app-text)]">转存写入权限</div>
      <p className="mb-3 text-xs text-[var(--app-muted)]">
        挂载/扫描只需要读取；只有手动开启这里，才允许把本地文件转存写入到该外部存储。可随时关闭。
      </p>
      <div className="flex flex-wrap items-center gap-4 text-sm text-[var(--app-subtle)]">
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            className="h-4 w-4 accent-primary-400"
            checked={transferEnabled}
            onChange={(event) => onTransferEnabledChange(event.target.checked)}
          />
          允许转存写入
        </label>
        <label className="flex items-center gap-2">
          <span>转存方式</span>
          <select
            className="input-base w-32"
            value={transferMode}
            onChange={(event) => onTransferModeChange(event.target.value === 'move' ? 'move' : 'copy')}
          >
            <option value="copy">复制</option>
            <option value="move">移动</option>
          </select>
        </label>
        <span className="text-xs text-[var(--app-muted)]">
          复制保留本地源文件；移动只在上传成功后删除本地文件。
        </span>
      </div>
    </div>
  )
}
