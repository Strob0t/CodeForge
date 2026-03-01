import { useNavigate, useSearchParams } from "@solidjs/router";
import { createSignal, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { Alert, Button, Card, ErrorBanner, FormField, Input } from "~/ui";

export default function ResetPasswordPage(): JSX.Element {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [password, setPassword] = createSignal("");
  const [confirmPassword, setConfirmPassword] = createSignal("");

  const token = (): string => {
    const t = searchParams.token;
    if (Array.isArray(t)) return t[0] ?? "";
    return t ?? "";
  };

  const { run, loading, error, clearError } = useAsyncAction(async () => {
    if (!token()) {
      throw new Error(t("auth.reset.invalidToken"));
    }
    if (password() !== confirmPassword()) {
      throw new Error(t("auth.reset.mismatch"));
    }
    await api.auth.resetPassword({
      token: token(),
      new_password: password(),
    });
    navigate("/login", { replace: true });
  });

  const handleSubmit = (e: SubmitEvent): void => {
    e.preventDefault();
    void run();
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <Card class="w-full max-w-sm">
        <Card.Body>
          <h1 class="mb-2 text-center text-2xl font-bold text-cf-text-primary">
            {t("auth.reset.title")}
          </h1>
          <p class="mb-6 text-center text-sm text-cf-text-secondary">
            {t("auth.reset.description")}
          </p>

          <Show when={!token()}>
            <Alert variant="error" class="mb-4">
              {t("auth.reset.invalidToken")}
            </Alert>
          </Show>

          <ErrorBanner error={error} onDismiss={clearError} />

          <form onSubmit={handleSubmit}>
            <FormField label={t("auth.reset.password")} id="reset_password" required class="mb-4">
              <Input
                id="reset_password"
                type="password"
                required
                value={password()}
                onInput={(e) => setPassword(e.currentTarget.value)}
                autocomplete="new-password"
              />
            </FormField>

            <FormField
              label={t("auth.reset.confirmPassword")}
              id="reset_confirm_password"
              required
              class="mb-6"
            >
              <Input
                id="reset_confirm_password"
                type="password"
                required
                value={confirmPassword()}
                onInput={(e) => setConfirmPassword(e.currentTarget.value)}
                autocomplete="new-password"
              />
            </FormField>

            <Button
              type="submit"
              variant="primary"
              loading={loading()}
              fullWidth
              disabled={!token()}
            >
              {loading() ? t("auth.reset.resetting") : t("auth.reset.submit")}
            </Button>
          </form>

          <a
            href="/login"
            class="mt-4 block text-center text-sm text-cf-text-muted hover:text-cf-text-secondary"
          >
            {t("auth.forgot.backToLogin")}
          </a>
        </Card.Body>
      </Card>
    </div>
  );
}
