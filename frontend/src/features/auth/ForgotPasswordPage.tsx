import { createSignal, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { Alert, Button, Card, ErrorBanner, FormField, Input } from "~/ui";

export default function ForgotPasswordPage(): JSX.Element {
  const { t } = useI18n();

  const [email, setEmail] = createSignal("");
  const [submitted, setSubmitted] = createSignal(false);

  const { run, loading, error, clearError } = useAsyncAction(async () => {
    await api.auth.forgotPassword({ email: email() });
    setSubmitted(true);
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
            {t("auth.forgot.title")}
          </h1>
          <p class="mb-6 text-center text-sm text-cf-text-secondary">
            {t("auth.forgot.description")}
          </p>

          <Show when={submitted()}>
            <Alert variant="info" class="mb-4">
              {t("auth.forgot.success")}
            </Alert>
          </Show>

          <ErrorBanner error={error} onDismiss={clearError} />

          <Show when={!submitted()}>
            <form onSubmit={handleSubmit}>
              <FormField label={t("auth.forgot.email")} id="forgot_email" required class="mb-6">
                <Input
                  id="forgot_email"
                  type="email"
                  required
                  value={email()}
                  onInput={(e) => setEmail(e.currentTarget.value)}
                  placeholder={t("auth.forgot.emailPlaceholder")}
                  autocomplete="email"
                />
              </FormField>

              <Button type="submit" variant="primary" loading={loading()} fullWidth>
                {loading() ? t("auth.forgot.sending") : t("auth.forgot.submit")}
              </Button>
            </form>
          </Show>

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
