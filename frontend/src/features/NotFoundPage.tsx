import { A } from "@solidjs/router";
import type { JSX } from "solid-js";

import { useI18n } from "~/i18n";

export default function NotFoundPage(): JSX.Element {
  const { t } = useI18n();

  return (
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-gray-100">{t("notFound.title")}</h2>
      <p class="mb-6 text-gray-500 dark:text-gray-400">{t("notFound.message")}</p>
      <A
        href="/"
        class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
      >
        {t("notFound.backToDashboard")}
      </A>
    </div>
  );
}
