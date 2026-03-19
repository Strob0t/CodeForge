export interface NotificationSettings {
  enablePush: boolean;
  enableSound: boolean;
  soundType: "default" | "subtle" | "none";
  notifyOn: {
    permissionRequest: boolean;
    runComplete: boolean;
    runFailed: boolean;
    agentMessage: boolean;
  };
}

const STORAGE_KEY = "codeforge_notification_settings";

const DEFAULTS: NotificationSettings = {
  enablePush: true,
  enableSound: false,
  soundType: "default",
  notifyOn: {
    permissionRequest: true,
    runComplete: true,
    runFailed: true,
    agentMessage: false,
  },
};

export function loadSettings(): NotificationSettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULTS, notifyOn: { ...DEFAULTS.notifyOn } };
    const parsed = JSON.parse(raw) as Partial<NotificationSettings>;
    return {
      ...DEFAULTS,
      ...parsed,
      notifyOn: { ...DEFAULTS.notifyOn, ...parsed.notifyOn },
    };
  } catch {
    return { ...DEFAULTS, notifyOn: { ...DEFAULTS.notifyOn } };
  }
}

export function saveSettings(settings: NotificationSettings): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
}
