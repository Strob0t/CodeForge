import { createResource, Show } from "solid-js";

import { api } from "~/api/client";
import type { User } from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Section, Table } from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

export default function UsersSection() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();
  const auth = useAuth();

  const [users, { refetch: refetchUsers }] = createResource<User[]>(() => {
    if (auth.user()?.role === "admin") return api.users.list();
    return Promise.resolve([]);
  });

  const handleDeleteUser = async (id: string) => {
    const ok = await confirm({
      title: t("common.delete"),
      message: t("settings.users.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.users.delete(id);
      refetchUsers();
      toast("success", t("settings.users.deleted"));
    } catch {
      toast("error", t("settings.users.deleteFailed"));
    }
  };

  const handleToggleUser = async (u: User) => {
    try {
      await api.users.update(u.id, { enabled: !u.enabled });
      refetchUsers();
    } catch {
      toast("error", t("settings.users.updateFailed"));
    }
  };

  const userColumns: TableColumn<User>[] = [
    { key: "email", header: t("settings.users.email") },
    { key: "name", header: t("settings.users.name") },
    {
      key: "role",
      header: t("settings.users.role"),
      render: (u) => (
        <Badge
          variant={u.role === "admin" ? "danger" : u.role === "editor" ? "primary" : "default"}
          pill
        >
          {u.role}
        </Badge>
      ),
    },
    {
      key: "status",
      header: t("common.status"),
      render: (u) => (
        <Button
          variant="ghost"
          size="xs"
          onClick={() => handleToggleUser(u)}
          aria-label={
            u.enabled
              ? t("settings.users.disableAria", { name: u.name })
              : t("settings.users.enableAria", { name: u.name })
          }
        >
          <Badge variant={u.enabled ? "success" : "default"} pill>
            {u.enabled ? t("settings.users.enabled") : t("settings.users.disabled")}
          </Badge>
        </Button>
      ),
    },
    {
      key: "actions",
      header: t("settings.users.actions"),
      render: (u) => (
        <Button
          variant="danger"
          size="sm"
          onClick={() => handleDeleteUser(u.id)}
          aria-label={t("settings.users.deleteAria", { name: u.name })}
        >
          {t("common.delete")}
        </Button>
      ),
    },
  ];

  return (
    <Show when={auth.user()?.role === "admin"}>
      <Section id="settings-users" title={t("settings.users.title")} class="mb-8">
        <Table<User>
          columns={userColumns}
          data={users() ?? []}
          rowKey={(u) => u.id}
          emptyMessage={t("settings.users.empty")}
        />
      </Section>
    </Show>
  );
}
