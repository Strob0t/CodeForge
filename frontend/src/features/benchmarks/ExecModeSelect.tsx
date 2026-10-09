import { For, type JSX } from "solid-js";

import type { BenchmarkExecMode } from "~/api/types";
import { useI18n } from "~/i18n";
import { FormField, Select } from "~/ui/primitives";

interface ExecModeOption {
  value: BenchmarkExecMode;
  label: string;
  /** Sandbox and hybrid stay listed as planned modes but cannot be picked until
   * tools actually run inside the container; today they would run unisolated (KI-13). */
  available: boolean;
}

export const EXEC_MODE_OPTIONS: readonly ExecModeOption[] = [
  { value: "mount", label: "Mount (direct file access)", available: true },
  { value: "sandbox", label: "Sandbox (isolated container)", available: false },
  { value: "hybrid", label: "Hybrid", available: false },
];

interface ExecModeSelectProps {
  value: BenchmarkExecMode;
  onChange: (mode: BenchmarkExecMode) => void;
}

export function ExecModeSelect(props: ExecModeSelectProps): JSX.Element {
  const { t } = useI18n();

  const handleChange = (value: string): void => {
    const option = EXEC_MODE_OPTIONS.find((o) => o.value === value);
    if (option?.available) props.onChange(option.value);
  };

  return (
    <FormField
      label={t("benchmark.execMode")}
      id="benchmark-exec-mode"
      help={t("benchmark.execModeUnavailableHelp")}
    >
      <Select
        id="benchmark-exec-mode"
        value={props.value}
        onChange={(e) => handleChange(e.currentTarget.value)}
      >
        <For each={EXEC_MODE_OPTIONS}>
          {(option) => (
            <option value={option.value} disabled={!option.available}>
              {option.available
                ? option.label
                : `${option.label} - ${t("benchmark.execModeUnavailable")}`}
            </option>
          )}
        </For>
      </Select>
    </FormField>
  );
}
