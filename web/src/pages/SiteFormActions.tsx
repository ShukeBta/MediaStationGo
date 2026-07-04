import { RefreshCw } from "lucide-react";

import type { SiteForm } from "./sitesPageModel";

type SiteFormActionsProps = {
  form: SiteForm;
  saving: boolean;
  onClose: () => void;
};

export function SiteFormActions({
  form,
  saving,
  onClose,
}: SiteFormActionsProps) {
  return (
    <div className="flex justify-end gap-2 pt-2">
      <button
        type="button"
        onClick={onClose}
        className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50 transition"
      >
        取消
      </button>
      <button
        type="submit"
        disabled={saving || !form.name.trim() || !form.url.trim()}
        className="neon-button text-sm disabled:opacity-50 flex items-center gap-1.5"
      >
        {saving ? (
          <>
            <RefreshCw size={14} className="animate-spin" />
            保存中...
          </>
        ) : (
          "保存"
        )}
      </button>
    </div>
  );
}
