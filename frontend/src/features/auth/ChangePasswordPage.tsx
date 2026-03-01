import { useNavigate } from "@solidjs/router";
import { createSignal, type JSX, Show } from "solid-js";

import { useAuth } from "~/components/AuthProvider";
import { useI18n } from "~/i18n";
import { Button, Card, ErrorBanner, FormField, Input } from "~/ui";

export default function ChangePasswordPage(): JSX.Element {
  const { t } = useI18n();
  const { changePassword, user, logout } = useAuth();
  const navigate = useNavigate();

  const [oldPassword, setOldPassword] = createSignal("");
  const [newPassword, setNewPassword] = createSignal("");
  const [confirmPassword, setConfirmPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(false);

  const handleSubmit = async (e: SubmitEvent): Promise<void> => {
    e.preventDefault();
    setError("");

    if (newPassword() !== confirmPassword()) {
      setError(t("auth.cp.mismatch"));
      return;
    }

    setLoading(true);
    try {
      await changePassword(oldPassword(), newPassword());
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.cp.failed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <Card class="w-full max-w-sm">
        <Card.Body>
          <h1 class="mb-2 text-center text-2xl font-bold text-cf-text-primary">
            {t("auth.cp.title")}
          </h1>
          <p class="mb-6 text-center text-sm text-cf-text-secondary">{t("auth.cp.description")}</p>

          <ErrorBanner error={error} onDismiss={() => setError("")} />

          <form onSubmit={handleSubmit}>
            <Show when={user()?.email}>
              <p class="mb-4 text-center text-xs text-cf-text-muted">{user()?.email}</p>
            </Show>

            <FormField label={t("auth.cp.currentPassword")} id="old_password" required class="mb-4">
              <Input
                id="old_password"
                type="password"
                required
                value={oldPassword()}
                onInput={(e) => setOldPassword(e.currentTarget.value)}
                autocomplete="current-password"
              />
            </FormField>

            <FormField label={t("auth.cp.newPassword")} id="new_password" required class="mb-4">
              <Input
                id="new_password"
                type="password"
                required
                value={newPassword()}
                onInput={(e) => setNewPassword(e.currentTarget.value)}
                autocomplete="new-password"
              />
            </FormField>

            <FormField
              label={t("auth.cp.confirmPassword")}
              id="confirm_password"
              required
              class="mb-6"
            >
              <Input
                id="confirm_password"
                type="password"
                required
                value={confirmPassword()}
                onInput={(e) => setConfirmPassword(e.currentTarget.value)}
                autocomplete="new-password"
              />
            </FormField>

            <Button type="submit" variant="primary" loading={loading()} fullWidth>
              {loading() ? t("auth.cp.changing") : t("auth.cp.submit")}
            </Button>
          </form>

          <button
            type="button"
            onClick={() => void logout()}
            class="mt-4 w-full text-center text-sm text-cf-text-muted hover:text-cf-text-secondary"
          >
            {t("auth.logout")}
          </button>
        </Card.Body>
      </Card>
    </div>
  );
}
