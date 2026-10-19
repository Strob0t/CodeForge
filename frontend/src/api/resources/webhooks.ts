import type { CoreClient } from "../core";
import { url } from "../factory";
import type { CreateWebhookRequest, WebhookEndpoint, WebhookRegistered } from "../types";

/**
 * The inbound webhooks of a project (KI-85): admins register, rotate and
 * delete them, editors list them. Changes are sent once (no retry, no
 * offline queue): a retried registration gets 409 and loses the secret of
 * the first answer, a retried rotation rotates again, and the queue would
 * keep secrets and tokens in memory until the browser is back online.
 */
export function createWebhooksResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<WebhookEndpoint[]>(url`/projects/${projectId}/webhooks`),

    /** The answer carries the webhook's secret, the only time it is shown. */
    create: (projectId: string, req: CreateWebhookRequest) =>
      c.requestOnce<WebhookRegistered>(url`/projects/${projectId}/webhooks`, {
        method: "POST",
        body: JSON.stringify(req),
      }),

    /**
     * A new secret, shown once; the old one stops working. secret is the one
     * Plane regenerated (GitHub and GitLab webhooks get a random one).
     */
    rotate: (projectId: string, webhookId: string, secret?: string) =>
      c.requestOnce<WebhookRegistered>(url`/projects/${projectId}/webhooks/${webhookId}/rotate`, {
        method: "POST",
        body: secret ? JSON.stringify({ secret }) : undefined,
      }),

    /** A PM webhook's API token ("" removes it). */
    setAPIToken: (projectId: string, webhookId: string, apiToken: string) =>
      c.requestOnce<undefined>(url`/projects/${projectId}/webhooks/${webhookId}/api-token`, {
        method: "PUT",
        body: JSON.stringify({ api_token: apiToken }),
      }),

    delete: (projectId: string, webhookId: string) =>
      c.requestOnce<undefined>(url`/projects/${projectId}/webhooks/${webhookId}`, {
        method: "DELETE",
      }),
  };
}
