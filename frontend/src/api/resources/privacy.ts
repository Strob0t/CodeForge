import type { CoreClient } from "../core";
import { url } from "../factory";
import type { ConsentPurpose, ConsentStatus, UserDataExport } from "../types";

const EXPORT_PATH = "/me/export";

/** GDPR self-service of the signed-in user: export, erasure and consent. */
export function createPrivacyResource(c: CoreClient) {
  return {
    /**
     * All personal data of the user. The client caches GET responses; the
     * export must not stay in memory once it is saved, nor be served again.
     */
    exportMyData: async (): Promise<UserDataExport> => {
      try {
        return await c.get<UserDataExport>(EXPORT_PATH);
      } finally {
        c.invalidateCache(EXPORT_PATH);
      }
    },

    /**
     * Erases the user: the account is gone afterwards. Sent once: a retry
     * after a lost answer would only fail, and the offline queue would hold
     * the request until the browser is back online.
     */
    deleteMyData: () => c.requestOnce<undefined>("/me/data", { method: "DELETE" }),

    consentPurposes: () => c.get<ConsentPurpose[]>("/me/consent/purposes"),

    consentStatus: () => c.get<ConsentStatus[]>("/me/consent"),

    setConsent: (purposeId: string, granted: boolean) =>
      c.put<undefined>(url`/me/consent/${purposeId}`, { granted }),
  };
}
