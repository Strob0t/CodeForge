import { useNavigate } from "@solidjs/router";
import { createSignal, type JSX } from "solid-js";

import { useAuth } from "~/components/AuthProvider";
import { useI18n } from "~/i18n";

export default function LoginPage(): JSX.Element {
  const { t } = useI18n();
  const { login } = useAuth();
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
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.loginFailed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
      <div class="w-full max-w-sm rounded-lg border border-gray-200 bg-white p-8 shadow-md dark:border-gray-700 dark:bg-gray-800">
        <h1 class="mb-6 text-center text-2xl font-bold text-gray-900 dark:text-gray-100">
          {t("auth.title")}
        </h1>

        {error() && (
          <div
            class="mb-4 rounded border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400"
            role="alert"
          >
            {error()}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div class="mb-4">
            <label
              for="email"
              class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {t("auth.email")}
            </label>
            <input
              id="email"
              type="email"
              required
              value={email()}
              onInput={(e) => setEmail(e.currentTarget.value)}
              class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
              placeholder={t("auth.emailPlaceholder")}
              autocomplete="email"
            />
          </div>

          <div class="mb-6">
            <label
              for="password"
              class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {t("auth.password")}
            </label>
            <input
              id="password"
              type="password"
              required
              value={password()}
              onInput={(e) => setPassword(e.currentTarget.value)}
              class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
              placeholder={t("auth.passwordPlaceholder")}
              autocomplete="current-password"
            />
          </div>

          <button
            type="submit"
            disabled={loading()}
            class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus:ring-offset-gray-800"
          >
            {loading() ? t("auth.loggingIn") : t("auth.login")}
          </button>
        </form>
      </div>
    </div>
  );
}
