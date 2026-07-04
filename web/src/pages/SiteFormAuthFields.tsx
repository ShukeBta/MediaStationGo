import type { SiteFormSectionProps } from "./siteFormModalTypes";

type SiteFormAuthFieldsProps = SiteFormSectionProps & {
  editingId: string | null;
};

const authOptions = [
  { value: "cookie", label: "Cookie" },
  { value: "api_key", label: "API Key" },
  { value: "auth_header", label: "Auth Header" },
];

export function SiteFormAuthFields({
  editingId,
  form,
  setForm,
}: SiteFormAuthFieldsProps) {
  return (
    <div>
      <label className="block text-sm text-ink-50 mb-2">认证方式</label>
      <div className="flex gap-2 mb-3">
        {authOptions.map((option) => (
          <button
            key={option.value}
            type="button"
            onClick={() =>
              setForm((current) => ({
                ...current,
                auth_type: option.value,
              }))
            }
            className={`px-3 py-1.5 rounded-xl text-xs font-medium border transition ${
              form.auth_type === option.value
                ? "bg-primary-500 text-ink-600 border-primary-500"
                : "border-gray-200 text-ink-50 hover:border-primary-500/50"
            }`}
          >
            {option.label}
          </button>
        ))}
      </div>

      {form.auth_type === "cookie" && (
        <div>
          <label className="block text-xs text-ink-50 mb-1">Cookie</label>
          <textarea
            rows={3}
            className="input-base w-full resize-none text-xs font-mono"
            placeholder="uid=xxx; pass=xxx; ..."
            value={form.cookie}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                cookie: event.target.value,
              }))
            }
          />
          <p className="text-xs text-sand-500 mt-1">
            从浏览器开发者工具的请求头中获取 Cookie 值
          </p>
        </div>
      )}

      {form.auth_type === "api_key" && (
        <div>
          <label className="block text-xs text-ink-50 mb-1">
            令牌（API Key / Passkey）
          </label>
          <input
            type="password"
            className="input-base w-full font-mono text-sm"
            placeholder={
              editingId
                ? "留空则保留原令牌，输入新值则替换"
                : "输入 API Access Token"
            }
            value={form.api_key}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                api_key: event.target.value,
              }))
            }
          />
          <p className="text-xs text-sand-500 mt-1">
            {form.type === "mteam"
              ? "馒头：控制台 → 实验室 → 存取令牌；第三方工具通过 x-api-key 请求头访问"
              : form.type === "yemapt"
                ? "YemaPT：个人详情页 → 第三方对接专用 auth；通过 Authorization 请求头原样访问"
                : "站点的访问 API Key"}
          </p>
        </div>
      )}

      {form.auth_type === "auth_header" && (
        <div>
          <label className="block text-xs text-ink-50 mb-1">
            请求头（Authorization）
          </label>
          <input
            className="input-base w-full font-mono text-xs"
            placeholder="Bearer eyJhbGciOiJIUzI1NiIs..."
            value={form.auth_header}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                auth_header: event.target.value,
              }))
            }
          />
        </div>
      )}

      <div className="mt-4">
        <label className="block text-xs text-ink-50 mb-1">RSS 地址</label>
        <input
          className="input-base w-full text-xs font-mono"
          placeholder="https://.../torrents/rss?..."
          value={form.rss_url}
          onChange={(event) =>
            setForm((current) => ({
              ...current,
              rss_url: event.target.value,
            }))
          }
        />
        <p className="text-xs text-sand-500 mt-1">
          站点 RSS 订阅地址，用于获取最新资源
        </p>
      </div>
    </div>
  );
}
