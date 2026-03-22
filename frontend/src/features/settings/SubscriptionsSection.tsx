import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { DeviceFlowResponse, SubscriptionProvider } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Section } from "~/ui";

export default function SubscriptionsSection() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();

  const [subProviders, { refetch: refetchSubProviders }] = createResource(() =>
    api.subscriptionProviders.list().then((r) => r.providers),
  );
  const [deviceFlow, setDeviceFlow] = createSignal<{
    provider: string;
    data: DeviceFlowResponse;
  } | null>(null);
  const [connectingProvider, setConnectingProvider] = createSignal<string | null>(null);
  const [pollTimer, setPollTimer] = createSignal<ReturnType<typeof setInterval> | null>(null);

  onCleanup(() => {
    const timer = pollTimer();
    if (timer) clearInterval(timer);
  });

  const handleConnectProvider = async (provider: SubscriptionProvider) => {
    setConnectingProvider(provider.name);
    try {
      const data = await api.subscriptionProviders.connect(provider.name);
      setDeviceFlow({ provider: provider.name, data });
      // Start polling for status
      const interval = Math.max(data.interval, 2) * 1000;
      const timer = setInterval(async () => {
        try {
          const result = await api.subscriptionProviders.status(provider.name);
          if (result.status === "complete") {
            clearInterval(timer);
            setPollTimer(null);
            setDeviceFlow(null);
            setConnectingProvider(null);
            refetchSubProviders();
            toast("success", t("settings.subscriptionProviders.connectSuccess"));
          } else if (result.status === "error") {
            clearInterval(timer);
            setPollTimer(null);
            setDeviceFlow(null);
            setConnectingProvider(null);
            toast("error", result.error ?? t("settings.subscriptionProviders.connectFailed"));
          }
        } catch {
          clearInterval(timer);
          setPollTimer(null);
          setDeviceFlow(null);
          setConnectingProvider(null);
          toast("error", t("settings.subscriptionProviders.connectFailed"));
        }
      }, interval);
      setPollTimer(timer);
    } catch {
      setConnectingProvider(null);
      toast("error", t("settings.subscriptionProviders.connectFailed"));
    }
  };

  const handleDisconnectProvider = async (provider: SubscriptionProvider) => {
    const ok = await confirm({
      title: t("settings.subscriptionProviders.disconnect"),
      message: t("settings.subscriptionProviders.confirmDisconnect"),
      variant: "danger",
      confirmLabel: t("settings.subscriptionProviders.disconnect"),
    });
    if (!ok) return;
    try {
      await api.subscriptionProviders.disconnect(provider.name);
      refetchSubProviders();
      toast("success", t("settings.subscriptionProviders.disconnectSuccess"));
    } catch {
      toast("error", t("settings.subscriptionProviders.disconnectFailed"));
    }
  };

  return (
    <Section
      id="settings-subscriptions"
      title={t("settings.subscriptionProviders.title")}
      class="mb-8"
    >
      <p class="mb-4 text-sm text-cf-text-muted">{t("settings.subscriptionProviders.subtitle")}</p>
      <Show
        when={!subProviders.loading}
        fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
      >
        <Show
          when={(subProviders() ?? []).length > 0}
          fallback={
            <p class="text-sm text-cf-text-muted">{t("settings.subscriptionProviders.empty")}</p>
          }
        >
          <div class="space-y-4">
            <For each={subProviders() ?? []}>
              {(provider) => (
                <Card>
                  <Card.Body>
                    <div class="flex items-start justify-between">
                      <div class="flex-1">
                        <div class="flex items-center gap-2">
                          <h4 class="text-sm font-medium text-cf-text-primary">
                            {provider.display_name}
                          </h4>
                          <Badge variant={provider.connected ? "success" : "default"} pill>
                            {provider.connected
                              ? t("settings.subscriptionProviders.connected")
                              : t("settings.subscriptionProviders.disconnected")}
                          </Badge>
                        </div>
                        <p class="mt-1 text-xs text-cf-text-muted">{provider.description}</p>
                        <div class="mt-2">
                          <span class="text-xs font-medium text-cf-text-tertiary">
                            {t("settings.subscriptionProviders.models")}:
                          </span>
                          <div class="mt-1 flex flex-wrap gap-1">
                            <For each={provider.models}>
                              {(model) => (
                                <Badge variant="info" pill>
                                  {model}
                                </Badge>
                              )}
                            </For>
                          </div>
                        </div>
                      </div>
                      <div class="ml-4 flex-shrink-0">
                        <Show
                          when={provider.connected}
                          fallback={
                            <Button
                              variant="primary"
                              size="sm"
                              onClick={() => handleConnectProvider(provider)}
                              loading={connectingProvider() === provider.name}
                              disabled={connectingProvider() !== null}
                            >
                              {connectingProvider() === provider.name
                                ? t("settings.subscriptionProviders.connecting")
                                : t("settings.subscriptionProviders.connect")}
                            </Button>
                          }
                        >
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={() => handleDisconnectProvider(provider)}
                          >
                            {t("settings.subscriptionProviders.disconnect")}
                          </Button>
                        </Show>
                      </div>
                    </div>

                    {/* Device flow UI */}
                    <Show
                      when={deviceFlow()?.provider === provider.name ? deviceFlow() : undefined}
                    >
                      {(flow) => (
                        <div class="mt-4 rounded-lg border border-cf-border bg-cf-bg-surface-alt p-4">
                          <p class="mb-2 text-sm font-medium text-cf-text-secondary">
                            {t("settings.subscriptionProviders.deviceCode")}:
                          </p>
                          <p class="mb-3 select-all text-center font-mono text-2xl font-bold tracking-widest text-cf-text-primary">
                            {flow().data.user_code}
                          </p>
                          <div class="flex items-center justify-center gap-2">
                            <a
                              href={flow().data.verification_uri}
                              target="_blank"
                              rel="noopener noreferrer"
                              class="text-sm font-medium text-cf-primary-fg underline hover:text-cf-primary-hover"
                            >
                              {t("settings.subscriptionProviders.openBrowser")}
                            </a>
                          </div>
                          <p class="mt-3 text-center text-xs text-cf-text-muted">
                            {t("settings.subscriptionProviders.waitingAuth")}
                          </p>
                        </div>
                      )}
                    </Show>
                  </Card.Body>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </Section>
  );
}
