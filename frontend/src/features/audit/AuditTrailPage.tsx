import { useI18n } from "~/i18n";
import { PageLayout } from "~/ui";

import AuditTable from "./AuditTable";

export function AuditContent() {
  return <AuditTable />;
}

export default function AuditTrailPage() {
  const { t } = useI18n();
  return (
    <PageLayout title={t("audit.title")} description={t("audit.description")}>
      <AuditContent />
    </PageLayout>
  );
}
