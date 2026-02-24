import { For, type JSX, splitProps } from "solid-js";

export interface TabItem {
  value: string;
  label: string;
  disabled?: boolean;
}

export type TabsVariant = "underline" | "pills";

export interface TabsProps {
  items: TabItem[];
  value: string;
  onChange: (value: string) => void;
  variant?: TabsVariant;
  class?: string;
}

export function Tabs(props: TabsProps): JSX.Element {
  const [local] = splitProps(props, ["items", "value", "onChange", "variant", "class"]);

  const variant = (): TabsVariant => local.variant ?? "underline";

  const baseClasses: Record<TabsVariant, string> = {
    underline: "flex border-b border-cf-border",
    pills: "flex gap-1 rounded-cf-md bg-cf-bg-surface-alt p-1",
  };

  const itemBase: Record<TabsVariant, string> = {
    underline: "px-4 py-2 text-sm font-medium transition-colors -mb-px border-b-2",
    pills: "px-3 py-1.5 text-sm font-medium rounded-cf-sm transition-colors",
  };

  function itemClass(item: TabItem): string {
    const isActive = item.value === local.value;
    const v = variant();

    if (item.disabled) {
      return itemBase[v] + " text-cf-text-muted cursor-not-allowed";
    }

    if (v === "underline") {
      return (
        itemBase[v] +
        (isActive
          ? " border-cf-accent text-cf-accent"
          : " border-transparent text-cf-text-tertiary hover:text-cf-text-secondary hover:border-cf-border")
      );
    }

    // pills
    return (
      itemBase[v] +
      (isActive
        ? " bg-cf-bg-surface text-cf-text-primary shadow-cf-sm"
        : " text-cf-text-tertiary hover:text-cf-text-secondary")
    );
  }

  return (
    <div role="tablist" class={baseClasses[variant()] + (local.class ? " " + local.class : "")}>
      <For each={local.items}>
        {(item) => (
          <button
            type="button"
            role="tab"
            aria-selected={item.value === local.value}
            disabled={item.disabled}
            class={itemClass(item)}
            onClick={() => {
              if (!item.disabled) local.onChange(item.value);
            }}
          >
            {item.label}
          </button>
        )}
      </For>
    </div>
  );
}
