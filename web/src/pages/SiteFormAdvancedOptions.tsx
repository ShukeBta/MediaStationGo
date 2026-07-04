import type { SiteFormSectionProps } from "./siteFormModalTypes";

type SiteFormAdvancedOptionsProps = SiteFormSectionProps & {
  advancedOpen: boolean;
  setAdvancedOpen: (open: boolean) => void;
};

export function SiteFormAdvancedOptions({
  advancedOpen,
  setAdvancedOpen,
  form,
  setForm,
}: SiteFormAdvancedOptionsProps) {
  return (
    <div>
      <button
        type="button"
        onClick={() => setAdvancedOpen(!advancedOpen)}
        className="flex items-center gap-1 text-xs text-ink-50 hover:text-white transition"
      >
        {advancedOpen ? "▾" : "▸"} 高级选项
      </button>
      {advancedOpen && (
        <div className="mt-3 pl-4 space-y-3 border-l border-gray-200">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-ink-50 mb-1">
                User-Agent
              </label>
              <input
                className="input-base w-full text-xs"
                placeholder="自定义 UA，留空使用默认"
                value={form.user_agent}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    user_agent: event.target.value,
                  }))
                }
              />
            </div>
            <div>
              <label className="block text-xs text-ink-50 mb-1">
                请求超时 (秒)
              </label>
              <input
                type="number"
                className="input-base w-full"
                min={1}
                max={300}
                value={form.timeout}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    timeout: Number(event.target.value),
                  }))
                }
              />
            </div>
            <div>
              <label className="block text-xs text-ink-50 mb-1">
                优先级 (数字越大越优先)
              </label>
              <input
                type="number"
                className="input-base w-full"
                min={1}
                max={100}
                value={form.priority}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    priority: Number(event.target.value),
                  }))
                }
              />
            </div>
            <div>
              <label className="block text-xs text-ink-50 mb-1">
                关联下载器
              </label>
              <input
                className="input-base w-full text-xs"
                placeholder="下载器 ID 或名称"
                value={form.downloader}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    downloader: event.target.value,
                  }))
                }
              />
            </div>
          </div>

          <div className="flex flex-wrap gap-4">
            <AdvancedCheckbox
              checked={form.use_proxy}
              label="使用代理"
              onChange={(checked) =>
                setForm((current) => ({ ...current, use_proxy: checked }))
              }
            />
            <AdvancedCheckbox
              checked={form.rate_limit}
              label="启用限流"
              onChange={(checked) =>
                setForm((current) => ({ ...current, rate_limit: checked }))
              }
            />
            <AdvancedCheckbox
              checked={form.browser_emulation}
              label="浏览器模拟"
              onChange={(checked) =>
                setForm((current) => ({
                  ...current,
                  browser_emulation: checked,
                }))
              }
            />
          </div>

          <div>
            <label className="block text-xs text-ink-50 mb-1">
              Extra 扩展配置 (JSON)
            </label>
            <textarea
              rows={3}
              className="input-base w-full resize-none text-xs font-mono"
              placeholder='{"key":"value"}'
              value={form.extra}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  extra: event.target.value,
                }))
              }
            />
          </div>
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() =>
                setForm((current) => ({
                  ...current,
                  is_default: !current.is_default,
                }))
              }
              className={`relative inline-flex h-5 w-9 shrink-0 rounded-full transition-colors cursor-pointer ${form.is_default ? "bg-primary-500" : "bg-gray-200"}`}
            >
              <span
                className={`pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow transform transition-transform mt-0.5 ${form.is_default ? "translate-x-4" : "translate-x-0.5"}`}
              />
            </button>
            <span className="text-sm text-ink-50">设为默认站点</span>
          </div>
        </div>
      )}
    </div>
  );
}

function AdvancedCheckbox({
  checked,
  label,
  onChange,
}: {
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        className="h-4 w-4 accent-primary-400"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="text-xs text-ink-100">{label}</span>
    </label>
  );
}
