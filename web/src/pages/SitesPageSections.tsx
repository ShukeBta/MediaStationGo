import { Globe, Plus, RefreshCw } from "lucide-react";

import { ManagementShortcuts } from "../components/ManagementShortcuts";
import type { Site } from "../types";
import { SiteCard } from "./SiteCard";

export function SitesManagementShortcuts() {
  return (
    <ManagementShortcuts
      title="站点与下载链路"
      description="把站点、搜索、订阅和下载器放在同一工作流里，避免功能入口被隐藏。"
      items={[
        {
          to: "/download-clients",
          title: "下载器管理",
          description: "添加、测试和维护下载器连接",
          badge: "必需",
          group: "基础配置",
        },
        {
          to: "/site-search",
          title: "站点检索",
          description: "跨 PT 站点搜索资源并创建下载任务",
          group: "资源获取",
        },
        {
          to: "/subscriptions",
          title: "订阅管理",
          description: "管理追剧追番和自动下载规则",
          group: "资源获取",
        },
        {
          to: "/downloads",
          title: "下载中心",
          description: "查看下载任务状态和历史记录",
          group: "任务状态",
        },
      ]}
    />
  );
}

export function SitesPageHeader({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex items-center justify-between">
      <h1 className="font-display text-3xl font-bold text-ink-600">
        站点管理
      </h1>
      <button
        onClick={onCreate}
        className="neon-button flex items-center gap-2"
      >
        <Plus size={16} />
        添加站点
      </button>
    </div>
  );
}

export function SitesGrid({
  sites,
  loading,
  testingId,
  onTest,
  onEdit,
  onDelete,
}: {
  sites: Site[];
  loading: boolean;
  testingId: string | null;
  onTest: (id: string) => void;
  onEdit: (id: string) => void;
  onDelete: (site: Site) => void;
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {sites.map((site) => (
        <SiteCard
          key={site.id}
          site={site}
          testing={testingId === site.id}
          onTest={() => onTest(site.id)}
          onEdit={() => onEdit(site.id)}
          onDelete={() => onDelete(site)}
        />
      ))}

      {!loading && sites.length === 0 && <SitesEmptyState />}
      {loading && <SitesLoadingState />}
    </div>
  );
}

function SitesEmptyState() {
  return (
    <div className="col-span-full py-12 text-center text-ink-50">
      <Globe size={40} className="mx-auto mb-3 text-gray-500" />
      <p>暂无站点</p>
      <p className="text-sm mt-1 text-sand-500">
        点击「添加站点」添加 PT/BT 站点
      </p>
    </div>
  );
}

function SitesLoadingState() {
  return (
    <div className="col-span-full py-12 text-center text-ink-50">
      <RefreshCw size={24} className="mx-auto mb-3 animate-spin" />
      <p>加载中...</p>
    </div>
  );
}
