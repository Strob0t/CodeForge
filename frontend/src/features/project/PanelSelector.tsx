import { createSignal, For, onCleanup, onMount, Show } from "solid-js";
import { Portal } from "solid-js/web";

// ---------------------------------------------------------------------------
// Grouped Panel Selector (custom dropdown with optgroup-style headers + tooltips)
// ---------------------------------------------------------------------------

const PANEL_GROUPS = [
  {
    label: "Workflow",
    items: [
      { value: "plan", label: "Plan", tip: "Goals, Roadmap & Features" },
      { value: "execute", label: "Execute", tip: "War Room, Runs & Trajectory" },
    ],
  },
  {
    label: "Tools",
    items: [
      { value: "code", label: "Code", tip: "Files, RepoMap & Search" },
      { value: "govern", label: "Govern", tip: "Policy & Audit" },
    ],
  },
] as const;

export function PanelSelector(props: { value: string; onChange: (v: string) => void }) {
  const [open, setOpen] = createSignal(false);
  const [pos, setPos] = createSignal({ top: 0, left: 0 });
  let containerRef: HTMLDivElement | undefined;
  let btnRef: HTMLButtonElement | undefined;
  let dropdownRef: HTMLDivElement | undefined;

  const selectedLabel = () => {
    for (const g of PANEL_GROUPS) {
      for (const item of g.items) {
        if (item.value === props.value) return item.label;
      }
    }
    return "More panels...";
  };

  const handleSelect = (value: string) => {
    props.onChange(value);
    setOpen(false);
  };

  const toggleOpen = () => {
    if (!open() && btnRef) {
      const rect = btnRef.getBoundingClientRect();
      setPos({ top: rect.bottom + 4, left: rect.left });
    }
    setOpen(!open());
  };

  // Close on outside click (must check both button container and portal dropdown).
  // Use "click" instead of "mousedown" to avoid racing with option button handlers.
  const onDocClick = (e: MouseEvent) => {
    const target = e.target as Node;
    if (containerRef?.contains(target)) return;
    if (dropdownRef?.contains(target)) return;
    // Also check by role attribute in case ref isn't assigned yet (Portal timing).
    if ((target as HTMLElement).closest?.('[role="listbox"], [role="option"]')) return;
    setOpen(false);
  };
  onMount(() => document.addEventListener("click", onDocClick));
  onCleanup(() => document.removeEventListener("click", onDocClick));

  return (
    <div ref={containerRef} class="relative">
      <button
        ref={btnRef}
        type="button"
        class="h-8 rounded-md border border-cf-border bg-cf-bg px-2 pr-7 text-sm text-cf-text cursor-pointer focus:outline-none focus:ring-1 focus:ring-cf-accent text-left min-w-[140px]"
        style={{
          "background-image": `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E")`,
          "background-repeat": "no-repeat",
          "background-position": "right 0.5rem center",
        }}
        onClick={toggleOpen}
        aria-haspopup="listbox"
        aria-expanded={open()}
      >
        {props.value ? selectedLabel() : "More panels..."}
      </button>
      <Show when={open()}>
        <Portal>
          <div
            ref={dropdownRef}
            role="listbox"
            style={{
              position: "fixed",
              "z-index": "99999",
              top: `${pos().top}px`,
              left: `${pos().left}px`,
              "min-width": "260px",
              "max-height": "70vh",
              "overflow-y": "auto",
              "border-radius": "0.5rem",
              border: "1px solid var(--cf-border)",
              "background-color": "var(--cf-bg-surface)",
              "box-shadow": "0 10px 25px rgba(0,0,0,0.15)",
              padding: "0.25rem 0",
            }}
          >
            <For each={PANEL_GROUPS}>
              {(group) => (
                <>
                  <div class="px-3 pt-2.5 pb-1 text-[10px] font-bold uppercase tracking-wider text-cf-text-tertiary select-none">
                    {group.label}
                  </div>
                  <For each={group.items}>
                    {(item) => (
                      <button
                        type="button"
                        role="option"
                        aria-selected={props.value === item.value}
                        class={`w-full text-left px-3 py-1.5 text-sm cursor-pointer hover:bg-cf-accent/10 flex flex-col gap-0 ${props.value === item.value ? "bg-cf-accent/15 text-cf-accent font-medium" : "text-cf-text"}`}
                        on:click={() => handleSelect(item.value)}
                        title={item.tip}
                      >
                        <span>{item.label}</span>
                        <span class="text-[11px] text-cf-text-tertiary leading-tight">
                          {item.tip}
                        </span>
                      </button>
                    )}
                  </For>
                </>
              )}
            </For>
          </div>
        </Portal>
      </Show>
    </div>
  );
}
