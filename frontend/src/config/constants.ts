// Layout constants (ProjectDetailPage split pane)
export const SPLIT_RATIO_KEY = "codeforge-split-ratio";
export const ROADMAP_COLLAPSED_KEY = "codeforge-roadmap-collapsed";
export const DEFAULT_SPLIT = 50;
export const MIN_SPLIT = 20;
export const MAX_SPLIT = 80;

// Toast constants
export const MAX_VISIBLE_TOASTS = 3;
export const DEFAULT_DISMISS_MS = 5000;

// API retry constants
export const MAX_RETRIES = 3;
export const RETRY_BASE_MS = 1000;
export const RETRYABLE_STATUSES = new Set([502, 503, 504]);
