import type { Dispatch, FormEvent, SetStateAction } from "react";
import { X } from "lucide-react";

import type { SiteForm } from "./sitesPageModel";
import { SiteFormActions } from "./SiteFormActions";
import { SiteFormAdvancedOptions } from "./SiteFormAdvancedOptions";
import { SiteFormAuthFields } from "./SiteFormAuthFields";
import { SiteFormBasicFields } from "./SiteFormBasicFields";

type SiteFormModalProps = {
  editingId: string | null;
  form: SiteForm;
  setForm: Dispatch<SetStateAction<SiteForm>>;
  saving: boolean;
  advancedOpen: boolean;
  setAdvancedOpen: Dispatch<SetStateAction<boolean>>;
  onClose: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void | Promise<void>;
  onTypeChange: (type: string) => void;
};

export function SiteFormModal({
  editingId,
  form,
  setForm,
  saving,
  advancedOpen,
  setAdvancedOpen,
  onClose,
  onSubmit,
  onTypeChange,
}: SiteFormModalProps) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="glass-panel w-full max-w-xl max-h-[75vh] overflow-y-auto mx-4 space-y-5"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-bold text-ink-600">
            {editingId ? "编辑站点" : "添加站点"}
          </h2>
          <button
            onClick={onClose}
            className="text-ink-50 hover:text-white transition"
          >
            <X size={20} />
          </button>
        </div>

        <form onSubmit={onSubmit} className="space-y-4">
          <SiteFormBasicFields
            form={form}
            setForm={setForm}
            onTypeChange={onTypeChange}
          />
          <SiteFormAuthFields
            editingId={editingId}
            form={form}
            setForm={setForm}
          />
          <SiteFormAdvancedOptions
            advancedOpen={advancedOpen}
            setAdvancedOpen={setAdvancedOpen}
            form={form}
            setForm={setForm}
          />
          <SiteFormActions form={form} saving={saving} onClose={onClose} />
        </form>
      </div>
    </div>
  );
}
