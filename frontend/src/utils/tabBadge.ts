const BASE_TITLE = "CodeForge";
let currentCount = 0;

export function updateTabBadge(count: number): void {
  currentCount = count;
  if (count > 0) {
    document.title = `(${count}) ${BASE_TITLE}`;
  } else {
    document.title = BASE_TITLE;
  }
}

export function resetTabBadge(): void {
  updateTabBadge(0);
}

export function getTabBadgeCount(): number {
  return currentCount;
}

// Auto-reset on window focus
if (typeof window !== "undefined") {
  window.addEventListener("focus", () => {
    resetTabBadge();
  });
}
