import { useNavigate } from "@solidjs/router";
import type { JSX } from "solid-js";

import { useI18n } from "~/i18n";
import { Button } from "~/ui";

export default function NotFoundPage(): JSX.Element {
  const { t } = useI18n();
  const navigate = useNavigate();

  return (
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <h2 class="mb-2 text-xl font-bold text-cf-text-primary">{t("notFound.title")}</h2>
      <p class="mb-6 text-cf-text-muted">{t("notFound.message")}</p>
      <Button variant="primary" onClick={() => navigate("/")}>
        {t("notFound.backToDashboard")}
      </Button>
    </div>
  );
}
