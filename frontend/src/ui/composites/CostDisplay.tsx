import type { JSX } from "solid-js";

import { useI18n } from "~/i18n";

export interface CostDisplayProps {
  /** Cost amount in USD. */
  usd: number;
  /** Additional CSS classes. */
  class?: string;
}

/**
 * Displays a cost value with 2-decimal summary and full-precision tooltip on hover.
 */
export function CostDisplay(props: CostDisplayProps): JSX.Element {
  const { fmt } = useI18n();

  return (
    <span class={props.class} title={fmt.currencyExact(props.usd)}>
      {fmt.currency(props.usd)}
    </span>
  );
}
