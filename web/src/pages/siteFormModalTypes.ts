import type { Dispatch, SetStateAction } from "react";

import type { SiteForm } from "./sitesPageModel";

export type SiteFormSetter = Dispatch<SetStateAction<SiteForm>>;

export type SiteFormSectionProps = {
  form: SiteForm;
  setForm: SiteFormSetter;
};
