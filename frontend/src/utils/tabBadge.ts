const BASE_TITLE = "CodeForge";

export function updateTabBadge(count: number): void {
  if (count > 0) {
    document.title = `(${count}) ${BASE_TITLE}`;
  } else {
    document.title = BASE_TITLE;
  }
}

export function resetTabBadge(): void {
  updateTabBadge(0);
}

// Auto-reset on window focus
if (typeof window !== "undefined") {
  window.addEventListener("focus", () => {
    resetTabBadge();
  });
}
