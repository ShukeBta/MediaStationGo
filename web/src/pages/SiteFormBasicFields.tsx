import type { SiteFormSectionProps } from "./siteFormModalTypes";

type SiteFormBasicFieldsProps = SiteFormSectionProps & {
  onTypeChange: (type: string) => void;
};

export function SiteFormBasicFields({
  form,
  setForm,
  onTypeChange,
}: SiteFormBasicFieldsProps) {
  return (
    <>
      <div>
        <label className="block text-sm text-ink-50 mb-1.5">
          站点名称 *
        </label>
        <input
          required
          className="input-base w-full"
          placeholder="例如: 馒头、观众、家园"
          value={form.name}
          onChange={(event) =>
            setForm((current) => ({ ...current, name: event.target.value }))
          }
        />
      </div>

      <div>
        <label className="block text-sm text-ink-50 mb-1.5">
          站点地址 *
        </label>
        <input
          required
          className="input-base w-full"
          placeholder="https://www.example.com/"
          value={form.url}
          onChange={(event) =>
            setForm((current) => ({ ...current, url: event.target.value }))
          }
        />
        <p className="text-xs text-sand-500 mt-1">
          格式: https://www.example.com/
        </p>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm text-ink-50 mb-1.5">
            站点类型
          </label>
          <select
            className="input-base w-full"
            value={form.type}
            onChange={(event) => onTypeChange(event.target.value)}
          >
            <option value="nexusphp">NexusPHP（国内主流PT）</option>
            <option value="gazelle">Gazelle（HDBits等）</option>
            <option value="unit3d">UNIT3D（BeyondHD等）</option>
            <option value="mteam">馒头 M-Team（专用API）</option>
            <option value="yemapt">YemaPT（API Auth）</option>
            <option value="discuz">Discuz 论坛型</option>
            <option value="custom_rss">自定义 RSS</option>
          </select>
        </div>
        <div>
          <label className="block text-sm text-ink-50 mb-1.5">状态</label>
          <div className="flex items-center gap-3 h-10">
            <button
              type="button"
              onClick={() =>
                setForm((current) => ({
                  ...current,
                  enabled: !current.enabled,
                }))
              }
              className={`relative inline-flex h-5 w-9 shrink-0 rounded-full transition-colors cursor-pointer ${form.enabled ? "bg-primary-500" : "bg-gray-200"}`}
            >
              <span
                className={`pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow transform transition-transform mt-0.5 ${form.enabled ? "translate-x-4" : "translate-x-0.5"}`}
              />
            </button>
            <span
              className={`text-sm ${form.enabled ? "text-white" : "text-sand-500"}`}
            >
              {form.enabled ? "启用" : "停用"}
            </span>
          </div>
        </div>
      </div>

      <SiteTypeGuide type={form.type} />
    </>
  );
}

function SiteTypeGuide({ type }: { type: string }) {
  if (type === "mteam") {
    return (
      <div className="p-3 rounded-xl border border-green-500/30 bg-green-500/5">
        <div className="text-sm font-medium text-green-400 mb-1">
          馒头站点配置指南
        </div>
        <div className="text-xs text-ink-50 space-y-1">
          <div>
            <b>认证方式：</b>必须使用「API Access Token」，不要使用 Cookie
          </div>
          <div className="pl-3 text-sand-500">
            1. 登录馒头站 → 控制台 → 实验室 → 存取令牌
            <br />
            2. 点击「创建令牌」，复制生成的 Token
            <br />
            3. 将 Token 填入下方「令牌」输入框
          </div>
        </div>
      </div>
    );
  }

  return null;
}
