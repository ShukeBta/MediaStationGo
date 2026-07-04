import type { Site } from "../types";

// ── 站点类型映射 ──
export const SITE_TYPE_LABELS: Record<string, string> = {
  nexusphp: "NexusPHP",
  gazelle: "Gazelle",
  unit3d: "UNIT3D",
  mteam: "M-Team",
  yemapt: "YemaPT",
  discuz: "Discuz",
  custom_rss: "自定义 RSS",
};

export const SITE_TYPE_ABBR: Record<string, string> = {
  nexusphp: "NP",
  gazelle: "GZ",
  unit3d: "U3",
  mteam: "MT",
  yemapt: "YM",
  discuz: "DZ",
  custom_rss: "RS",
};

export const SITE_TYPE_COLORS: Record<string, string> = {
  nexusphp: "bg-blue-500/15 text-blue-400",
  gazelle: "bg-purple-500/15 text-purple-400",
  unit3d: "bg-orange-500/15 text-orange-400",
  mteam: "bg-green-500/15 text-green-400",
  yemapt: "bg-cyan-500/15 text-cyan-400",
  discuz: "bg-yellow-500/15 text-yellow-400",
  custom_rss: "bg-sand-500/15 text-ink-50",
};

export const AUTH_TYPE_LABELS: Record<string, string> = {
  cookie: "Cookie",
  api_key: "API Key",
  auth_header: "Auth Header",
};

export const DEFAULT_SITE_TIMEOUT_SECONDS = 15;
export const API_SITE_TIMEOUT_SECONDS = 45;
export const API_SITE_TYPES = new Set(["mteam", "yemapt"]);

// ── 默认表单 ──
export const defaultSiteForm = () => ({
  name: "",
  url: "",
  type: "nexusphp",
  auth_type: "cookie",
  cookie: "",
  api_key: "",
  auth_header: "",
  enabled: true,
  is_default: false,
  extra: "",
  // 高级设置
  user_agent: "",
  rss_url: "",
  timeout: DEFAULT_SITE_TIMEOUT_SECONDS,
  priority: 50,
  use_proxy: false,
  rate_limit: false,
  browser_emulation: false,
  downloader: "",
});

export type SiteForm = ReturnType<typeof defaultSiteForm>;

export function siteToForm(site: Site): SiteForm {
  return {
    name: site.name || "",
    url: site.url || "",
    type: site.type || "nexusphp",
    auth_type: site.auth_type || "cookie",
    cookie: site.cookie || "",
    api_key: site.api_key || "",
    auth_header: site.auth_header || "",
    enabled: site.enabled !== false,
    is_default: site.is_default || false,
    extra: site.extra || "",
    user_agent: site.user_agent || "",
    rss_url: site.rss_url || "",
    timeout: site.timeout ?? DEFAULT_SITE_TIMEOUT_SECONDS,
    priority: site.priority ?? 50,
    use_proxy: site.use_proxy || false,
    rate_limit: site.rate_limit || false,
    browser_emulation: site.browser_emulation || false,
    downloader: site.downloader || "",
  };
}

export function siteFormToPayload(
  form: SiteForm,
  includeEmptySecrets: boolean,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    name: form.name.trim(),
    url: form.url.trim(),
    type: form.type,
    auth_type: form.auth_type,
    enabled: form.enabled,
    is_default: form.is_default,
    extra: form.extra || "",
    user_agent: form.user_agent || "",
    rss_url: form.rss_url || "",
    timeout:
      Number(form.timeout) ||
      (API_SITE_TYPES.has(form.type)
        ? API_SITE_TIMEOUT_SECONDS
        : DEFAULT_SITE_TIMEOUT_SECONDS),
    priority: Number(form.priority) || 50,
    use_proxy: !!form.use_proxy,
    rate_limit: !!form.rate_limit,
    browser_emulation: !!form.browser_emulation,
    downloader: form.downloader || "",
  };
  if (includeEmptySecrets || form.cookie.trim())
    payload.cookie = form.cookie.trim();
  if (includeEmptySecrets || form.api_key.trim())
    payload.api_key = form.api_key.trim();
  if (includeEmptySecrets || form.auth_header.trim())
    payload.auth_header = form.auth_header.trim();
  return payload;
}
