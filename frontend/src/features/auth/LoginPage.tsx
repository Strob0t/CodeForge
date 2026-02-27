import { useNavigate } from "@solidjs/router";
import { createSignal, type JSX, Show } from "solid-js";

import { useAuth } from "~/components/AuthProvider";
import { useI18n } from "~/i18n";
import { Alert, Button, Card, FormField, Input } from "~/ui";

export default function LoginPage(): JSX.Element {
  const { t } = useI18n();
  const { login, user } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(false);

  const handleSubmit = async (e: SubmitEvent): Promise<void> => {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await login(email(), password());
      // Redirect to change-password if backend requires it, otherwise dashboard.
      const target = user()?.must_change_password ? "/change-password" : "/";
      navigate(target, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.loginFailed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <Card class="w-full max-w-sm">
        <Card.Body>
          <h1 class="mb-6 text-center text-2xl font-bold text-cf-text-primary">
            {t("auth.title")}
          </h1>

          <Show when={error()}>
            <Alert variant="error" class="mb-4" onDismiss={() => setError("")}>
              {error()}
            </Alert>
          </Show>

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
        </Card.Body>
      </Card>
    </div>
  );
}
