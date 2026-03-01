import { useNavigate } from "@solidjs/router";
import { createSignal, type JSX } from "solid-js";

import { useAuth } from "~/components/AuthProvider";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { Button, Card, ErrorBanner, FormField, Input } from "~/ui";

export default function LoginPage(): JSX.Element {
  const { t } = useI18n();
  const { login, user } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = createSignal("");
  const [password, setPassword] = createSignal("");

  const { run, loading, error, clearError } = useAsyncAction(async () => {
    await login(email(), password());
    const target = user()?.must_change_password ? "/change-password" : "/";
    navigate(target, { replace: true });
  });

  const handleSubmit = (e: SubmitEvent): void => {
    e.preventDefault();
    void run();
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <Card class="w-full max-w-sm">
        <Card.Body>
          <h1 class="mb-6 text-center text-2xl font-bold text-cf-text-primary">
            {t("auth.title")}
          </h1>

          <ErrorBanner error={error} onDismiss={clearError} />

          <form onSubmit={handleSubmit}>
            <FormField label={t("auth.email")} id="email" required class="mb-4">
              <Input
                id="email"
                type="email"
                required
                value={email()}
                onInput={(e) => setEmail(e.currentTarget.value)}
                placeholder={t("auth.emailPlaceholder")}
                autocomplete="email"
              />
            </FormField>

            <FormField label={t("auth.password")} id="password" required class="mb-6">
              <Input
                id="password"
                type="password"
                required
                value={password()}
                onInput={(e) => setPassword(e.currentTarget.value)}
                placeholder={t("auth.passwordPlaceholder")}
                autocomplete="current-password"
              />
            </FormField>

            <Button type="submit" variant="primary" loading={loading()} fullWidth>
              {loading() ? t("auth.loggingIn") : t("auth.login")}
            </Button>
          </form>

          <a
            href="/forgot-password"
            class="mt-4 block text-center text-sm text-cf-text-muted hover:text-cf-text-secondary"
          >
            {t("auth.forgotPassword")}
          </a>
        </Card.Body>
      </Card>
    </div>
  );
}
