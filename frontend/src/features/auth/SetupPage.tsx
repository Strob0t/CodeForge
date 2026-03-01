import { useNavigate } from "@solidjs/router";
import { createSignal, type JSX, onCleanup, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import { useAuth } from "~/components/AuthProvider";
import { useI18n } from "~/i18n";
import { Alert, Button, Card, FormField, Input } from "~/ui";

export default function SetupPage(): JSX.Element {
  const { t } = useI18n();
  const { login } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = createSignal("admin@localhost");
  const [name, setName] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [confirmPassword, setConfirmPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(false);
  const [remainingSeconds, setRemainingSeconds] = createSignal(0);
  const [expired, setExpired] = createSignal(false);

  let timerInterval: ReturnType<typeof setInterval> | undefined;

  onMount(async () => {
    try {
      const status = await api.auth.setupStatus();
      if (!status.needs_setup) {
        navigate("/login", { replace: true });
        return;
      }
      setRemainingSeconds(status.setup_timeout_minutes * 60);

      timerInterval = setInterval(() => {
        setRemainingSeconds((prev) => {
          if (prev <= 1) {
            clearInterval(timerInterval);
            setExpired(true);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
    } catch {
      navigate("/login", { replace: true });
    }
  });

  onCleanup(() => {
    if (timerInterval) clearInterval(timerInterval);
  });

  const minutes = () => Math.floor(remainingSeconds() / 60);
  const seconds = () => remainingSeconds() % 60;

  const handleSubmit = async (e: SubmitEvent): Promise<void> => {
    e.preventDefault();
    setError("");

    if (password() !== confirmPassword()) {
      setError(t("auth.setup.mismatch"));
      return;
    }

    setLoading(true);
    try {
      await api.auth.setup({
        email: email(),
        name: name(),
        password: password(),
      });
      // Auto-login with the new credentials
      await login(email(), password());
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.setup.failed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <Card class="w-full max-w-sm">
        <Card.Body>
          <h1 class="mb-2 text-center text-2xl font-bold text-cf-text-primary">
            {t("auth.setup.title")}
          </h1>
          <p class="mb-6 text-center text-sm text-cf-text-secondary">
            {t("auth.setup.description")}
          </p>

          <Show when={expired()}>
            <Alert variant="error" class="mb-4">
              {t("auth.setup.expired")}
            </Alert>
          </Show>

          <Show when={!expired() && remainingSeconds() > 0}>
            <p class="mb-4 text-center text-xs text-cf-text-muted">
              {t("auth.setup.timeout", {
                minutes: String(minutes()),
                seconds: String(seconds()).padStart(2, "0"),
              })}
            </p>
          </Show>

          <Show when={error()}>
            <Alert variant="error" class="mb-4" onDismiss={() => setError("")}>
              {error()}
            </Alert>
          </Show>

          <form onSubmit={handleSubmit}>
            <FormField label={t("auth.setup.email")} id="setup_email" required class="mb-4">
              <Input
                id="setup_email"
                type="email"
                required
                value={email()}
                onInput={(e) => setEmail(e.currentTarget.value)}
                placeholder={t("auth.setup.emailPlaceholder")}
                autocomplete="email"
              />
            </FormField>

            <FormField label={t("auth.setup.name")} id="setup_name" required class="mb-4">
              <Input
                id="setup_name"
                type="text"
                required
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder={t("auth.setup.namePlaceholder")}
                autocomplete="name"
              />
            </FormField>

            <FormField label={t("auth.setup.password")} id="setup_password" required class="mb-4">
              <Input
                id="setup_password"
                type="password"
                required
                value={password()}
                onInput={(e) => setPassword(e.currentTarget.value)}
                autocomplete="new-password"
              />
            </FormField>

            <FormField
              label={t("auth.setup.confirmPassword")}
              id="setup_confirm_password"
              required
              class="mb-6"
            >
              <Input
                id="setup_confirm_password"
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
              disabled={expired()}
            >
              {loading() ? t("auth.setup.creating") : t("auth.setup.submit")}
            </Button>
          </form>
        </Card.Body>
      </Card>
    </div>
  );
}
