import { type FormEvent, useEffect, useState } from "react";
import toast from "react-hot-toast";

import { sitesAPI } from "../api/sites";
import type { Site } from "../types";
import { confirmAction } from "../components/confirmAction";

import {
  API_SITE_TIMEOUT_SECONDS,
  API_SITE_TYPES,
  DEFAULT_SITE_TIMEOUT_SECONDS,
  defaultSiteForm,
  siteFormToPayload,
  siteToForm,
} from "./sitesPageModel";
import { SiteFormModal } from "./SiteFormModal";
import {
  SitesGrid,
  SitesManagementShortcuts,
  SitesPageHeader,
} from "./SitesPageSections";

function apiErrorMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: { message?: string; error?: string } } })
    ?.response?.data;
  return data?.message || data?.error || (err instanceof Error ? err.message : fallback);
}

export function SitesPage() {
  const [sites, setSites] = useState<Site[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState(defaultSiteForm());
  const [saving, setSaving] = useState(false);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [advancedOpen, setAdvancedOpen] = useState(false);

  const loadSites = async () => {
    setLoading(true);
    try {
      const response = await sitesAPI.list();
      setSites(Array.isArray(response.data) ? response.data : []);
    } catch {
      toast.error("加载站点列表失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadSites();
  }, []);

  const openCreate = () => {
    setEditingId(null);
    setForm(defaultSiteForm());
    setAdvancedOpen(false);
    setShowModal(true);
  };

  const openEdit = async (id: string) => {
    try {
      const response = await sitesAPI.get(id);
      setEditingId(id);
      setForm(siteToForm(response.data as Site));
      setAdvancedOpen(false);
      setShowModal(true);
    } catch {
      toast.error("获取站点详情失败");
    }
  };

  const closeModal = () => {
    setShowModal(false);
    setEditingId(null);
  };

  const silentSave = async (): Promise<{ ok: boolean; message?: string }> => {
    if (!form.name.trim() || !form.url.trim()) {
      return { ok: false, message: "站点名称和地址不能为空" };
    }
    const payload = siteFormToPayload(form, !editingId);
    try {
      if (editingId) {
        await sitesAPI.update(editingId, payload);
      } else {
        const response = await sitesAPI.create(payload);
        setEditingId((response.data as Site)?.id ?? null);
      }
      return { ok: true };
    } catch (err: unknown) {
      return { ok: false, message: apiErrorMessage(err, "保存失败") };
    }
  };

  const handleSave = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSaving(true);
    const result = await silentSave();
    if (result.ok) {
      toast.success(editingId ? "站点已更新" : "站点已添加");
      closeModal();
      await loadSites();
    } else {
      toast.error(result.message || "保存失败");
    }
    setSaving(false);
  };

  const handleTest = async (id: string) => {
    if (editingId === id) {
      setSaving(true);
      const result = await silentSave();
      setSaving(false);
      if (!result.ok) {
        toast.error(result.message || "保存失败，无法测试");
        return;
      }
    }

    setTestingId(id);
    try {
      const response = await sitesAPI.test(id);
      toast.success(response?.message || "连接测试成功");
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, "连接测试失败"));
    } finally {
      setTestingId(null);
    }
  };

  const handleDelete = async (site: Site) => {
    const confirmed = await confirmAction({
      title: "删除站点",
      message: `确定要删除站点「${site.name}」吗？此操作不可撤销。`,
      confirmText: "删除",
    });
    if (!confirmed) return;

    try {
      await sitesAPI.remove(site.id);
      toast.success("站点已删除");
      await loadSites();
    } catch {
      toast.error("删除站点失败");
    }
  };

  const handleTypeChange = (type: string) => {
    const isAPISite = API_SITE_TYPES.has(type);
    setForm((current) => ({
      ...current,
      type,
      auth_type: isAPISite ? "api_key" : current.auth_type,
      timeout:
        isAPISite && current.timeout === DEFAULT_SITE_TIMEOUT_SECONDS
          ? API_SITE_TIMEOUT_SECONDS
          : current.timeout,
    }));
  };

  return (
    <div className="space-y-6">
      <SitesManagementShortcuts />
      <SitesPageHeader onCreate={openCreate} />
      <SitesGrid
        sites={sites}
        loading={loading}
        testingId={testingId}
        onTest={handleTest}
        onEdit={openEdit}
        onDelete={handleDelete}
      />

      {showModal && (
        <SiteFormModal
          editingId={editingId}
          form={form}
          setForm={setForm}
          saving={saving}
          advancedOpen={advancedOpen}
          setAdvancedOpen={setAdvancedOpen}
          onClose={closeModal}
          onSubmit={handleSave}
          onTypeChange={handleTypeChange}
        />
      )}
    </div>
  );
}
