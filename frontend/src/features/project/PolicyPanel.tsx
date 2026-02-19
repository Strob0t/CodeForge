import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  PermissionMode,
  PermissionRule,
  PolicyDecision,
  PolicyProfile,
  PolicyQualityGate,
  PolicyToolCall,
  TerminationCondition,
} from "~/api/types";
import { useI18n } from "~/i18n";

interface PolicyPanelProps {
  projectId: string;
  onError: (msg: string) => void;
}

type View = "list" | "detail" | "editor";

const PRESET_NAMES = new Set([
  "plan-readonly",
  "headless-safe-sandbox",
  "headless-permissive-sandbox",
  "trusted-mount-autonomous",
]);

const DECISION_COLORS: Record<PolicyDecision, string> = {
  allow: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  deny: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  ask: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
};

function emptyProfile(): PolicyProfile {
  return {
    name: "",
    description: "",
    mode: "default",
    rules: [],
    quality_gate: {
      require_tests_pass: false,
      require_lint_pass: false,
      rollback_on_gate_fail: false,
    },
    termination: {},
  };
}

function emptyRule(): PermissionRule {
  return {
    specifier: { tool: "" },
    decision: "allow",
  };
}

export default function PolicyPanel(props: PolicyPanelProps) {
  const { t } = useI18n();
  const [view, setView] = createSignal<View>("list");
  const [selectedName, setSelectedName] = createSignal<string | null>(null);
  const [profiles, { refetch: refetchProfiles }] = createResource(() => api.policies.list());
  const [selectedProfile, { refetch: refetchProfile }] = createResource(
    () => selectedName(),
    (name) => (name ? api.policies.get(name) : null),
  );

  const MODES = (): { value: PermissionMode; label: string }[] => [
    { value: "default", label: t("policy.mode.default") },
    { value: "acceptEdits", label: t("policy.mode.acceptEdits") },
    { value: "plan", label: t("policy.mode.plan") },
    { value: "delegate", label: t("policy.mode.delegate") },
  ];

  // Editor state
  const [editProfile, setEditProfile] = createSignal<PolicyProfile>(emptyProfile());
  const [saving, setSaving] = createSignal(false);

  // Evaluate tester state
  const [evalTool, setEvalTool] = createSignal("");
  const [evalCommand, setEvalCommand] = createSignal("");
  const [evalPath, setEvalPath] = createSignal("");
  const [evalResult, setEvalResult] = createSignal<PolicyDecision | null>(null);
  const [evaluating, setEvaluating] = createSignal(false);

  const handleSelect = (name: string) => {
    setSelectedName(name);
    setEvalResult(null);
    setView("detail");
    refetchProfile();
  };

  const handleNewPolicy = () => {
    setEditProfile(emptyProfile());
    setView("editor");
  };

  const handleClone = () => {
    const p = selectedProfile();
    if (!p) return;
    setEditProfile({ ...p, name: p.name + "-copy" });
    setView("editor");
  };

  const handleSave = async () => {
    const profile = editProfile();
    if (!profile.name) {
      props.onError(t("policy.toast.nameRequired"));
      return;
    }
    setSaving(true);
    props.onError("");
    try {
      await api.policies.create(profile);
      refetchProfiles();
      setSelectedName(profile.name);
      setView("detail");
      refetchProfile();
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("policy.toast.saveFailed"));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (name: string) => {
    props.onError("");
    try {
      await api.policies.delete(name);
      refetchProfiles();
      if (selectedName() === name) {
        setSelectedName(null);
        setView("list");
      }
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("policy.toast.deleteFailed"));
    }
  };

  const handleEvaluate = async () => {
    const name = selectedName();
    if (!name || !evalTool()) return;
    setEvaluating(true);
    setEvalResult(null);
    try {
      const call: PolicyToolCall = {
        tool: evalTool(),
        command: evalCommand() || undefined,
        path: evalPath() || undefined,
      };
      const res = await api.policies.evaluate(name, call);
      setEvalResult(res.decision);
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("policy.toast.evalFailed"));
    } finally {
      setEvaluating(false);
    }
  };

  const updateEditField = <K extends keyof PolicyProfile>(key: K, value: PolicyProfile[K]) => {
    setEditProfile((prev) => ({ ...prev, [key]: value }));
  };

  const updateTermination = <K extends keyof TerminationCondition>(
    key: K,
    value: TerminationCondition[K],
  ) => {
    setEditProfile((prev) => ({
      ...prev,
      termination: { ...prev.termination, [key]: value },
    }));
  };

  const updateQualityGate = <K extends keyof PolicyQualityGate>(
    key: K,
    value: PolicyQualityGate[K],
  ) => {
    setEditProfile((prev) => ({
      ...prev,
      quality_gate: { ...prev.quality_gate, [key]: value },
    }));
  };

  const addRule = () => {
    setEditProfile((prev) => ({
      ...prev,
      rules: [...prev.rules, emptyRule()],
    }));
  };

  const removeRule = (index: number) => {
    setEditProfile((prev) => ({
      ...prev,
      rules: prev.rules.filter((_, i) => i !== index),
    }));
  };

  const updateRule = (index: number, field: string, value: string) => {
    setEditProfile((prev) => {
      const rules = [...prev.rules];
      const rule = { ...rules[index] };
      if (field === "tool") {
        rule.specifier = { ...rule.specifier, tool: value };
      } else if (field === "sub_pattern") {
        rule.specifier = { ...rule.specifier, sub_pattern: value || undefined };
      } else if (field === "decision") {
        rule.decision = value as PolicyDecision;
      } else if (field === "path_allow") {
        rule.path_allow = value ? value.split(",").map((s) => s.trim()) : undefined;
      } else if (field === "path_deny") {
        rule.path_deny = value ? value.split(",").map((s) => s.trim()) : undefined;
      } else if (field === "command_allow") {
        rule.command_allow = value ? value.split(",").map((s) => s.trim()) : undefined;
      } else if (field === "command_deny") {
        rule.command_deny = value ? value.split(",").map((s) => s.trim()) : undefined;
      }
      rules[index] = rule;
      return { ...prev, rules };
    });
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      {/* Header */}
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("policy.title")}</h3>
        <div class="flex gap-2">
          <Show when={view() !== "list"}>
            <button
              class="rounded bg-gray-100 px-3 py-1.5 text-sm hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
              onClick={() => {
                setView("list");
                setSelectedName(null);
              }}
            >
              {t("policy.backToList")}
            </button>
          </Show>
          <Show when={view() === "list"}>
            <button
              class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
              onClick={handleNewPolicy}
            >
              {t("policy.newPolicy")}
            </button>
          </Show>
        </div>
      </div>

      {/* List View */}
      <Show when={view() === "list"}>
        <Show when={profiles.loading}>
          <p class="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>
        </Show>
        <Show when={!profiles.loading && profiles()}>
          <div class="space-y-1">
            <For each={profiles()?.profiles ?? []}>
              {(name) => (
                <div class="flex items-center justify-between rounded px-3 py-2 hover:bg-gray-50 dark:hover:bg-gray-700">
                  <button
                    class="flex items-center gap-2 text-sm font-medium text-gray-800 dark:text-gray-200"
                    onClick={() => handleSelect(name)}
                  >
                    <span>{name}</span>
                    <span
                      class={`rounded px-1.5 py-0.5 text-xs ${
                        PRESET_NAMES.has(name)
                          ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                          : "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
                      }`}
                    >
                      {PRESET_NAMES.has(name) ? t("policy.preset") : t("policy.custom")}
                    </span>
                  </button>
                  <Show when={!PRESET_NAMES.has(name)}>
                    <button
                      type="button"
                      class="rounded px-2 py-1 text-xs text-red-500 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                      onClick={() => handleDelete(name)}
                      aria-label={t("policy.deleteAria", { name })}
                    >
                      {t("common.delete")}
                    </button>
                  </Show>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Show>

      {/* Detail View */}
      <Show when={view() === "detail" && selectedProfile()}>
        {(p) => (
          <div>
            <div class="mb-4">
              <h4 class="text-base font-semibold">{p().name}</h4>
              <Show when={p().description}>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{p().description}</p>
              </Show>
            </div>

            {/* Summary */}
            <div class="mb-4 grid grid-cols-3 gap-3 text-sm">
              <div>
                <span class="text-gray-500 dark:text-gray-400">{t("policy.mode")}</span>{" "}
                <span class="font-medium">{p().mode}</span>
              </div>
              <Show when={p().termination.max_steps}>
                <div>
                  <span class="text-gray-500 dark:text-gray-400">{t("policy.steps")}</span>{" "}
                  <span class="font-medium">{p().termination.max_steps}</span>
                </div>
              </Show>
              <Show when={p().termination.timeout_seconds}>
                <div>
                  <span class="text-gray-500 dark:text-gray-400">{t("policy.timeout")}</span>{" "}
                  <span class="font-medium">{p().termination.timeout_seconds}s</span>
                </div>
              </Show>
              <Show when={p().termination.max_cost}>
                <div>
                  <span class="text-gray-500 dark:text-gray-400">{t("policy.cost")}</span>{" "}
                  <span class="font-medium">${p().termination.max_cost}</span>
                </div>
              </Show>
              <Show when={p().termination.stall_detection}>
                <div>
                  <span class="text-gray-500 dark:text-gray-400">{t("policy.stall")}</span>{" "}
                  <span class="font-medium">{p().termination.stall_threshold}</span>
                </div>
              </Show>
            </div>

            {/* Quality Gate */}
            <div class="mb-4">
              <h5 class="mb-1 text-sm font-medium text-gray-500 dark:text-gray-400">
                {t("policy.qualityGate")}
              </h5>
              <div class="flex gap-3 text-sm">
                <span
                  class={
                    p().quality_gate.require_tests_pass
                      ? "text-green-600"
                      : "text-gray-400 dark:text-gray-500"
                  }
                >
                  {p().quality_gate.require_tests_pass ? "\u2713" : "\u2717"} {t("policy.tests")}
                </span>
                <span
                  class={
                    p().quality_gate.require_lint_pass
                      ? "text-green-600"
                      : "text-gray-400 dark:text-gray-500"
                  }
                >
                  {p().quality_gate.require_lint_pass ? "\u2713" : "\u2717"} {t("policy.lint")}
                </span>
                <span
                  class={
                    p().quality_gate.rollback_on_gate_fail
                      ? "text-green-600"
                      : "text-gray-400 dark:text-gray-500"
                  }
                >
                  {p().quality_gate.rollback_on_gate_fail ? "\u2713" : "\u2717"}{" "}
                  {t("policy.rollback")}
                </span>
              </div>
            </div>

            {/* Rules Table */}
            <Show when={p().rules.length > 0}>
              <div class="mb-4">
                <h5 class="mb-1 text-sm font-medium text-gray-500 dark:text-gray-400">
                  {t("policy.rules")}
                </h5>
                <div class="overflow-x-auto">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b text-left text-xs text-gray-500 dark:border-gray-700 dark:text-gray-400">
                        <th class="pb-1 pr-3">{t("policy.table.tool")}</th>
                        <th class="pb-1 pr-3">{t("policy.table.pattern")}</th>
                        <th class="pb-1 pr-3">{t("policy.table.decision")}</th>
                        <th class="pb-1">{t("policy.table.constraints")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={p().rules}>
                        {(rule) => (
                          <tr class="border-b border-gray-100 dark:border-gray-700">
                            <td class="py-1 pr-3 font-mono">{rule.specifier.tool}</td>
                            <td class="py-1 pr-3 font-mono text-xs">
                              {rule.specifier.sub_pattern || ""}
                            </td>
                            <td class="py-1 pr-3">
                              <span
                                class={`rounded px-1.5 py-0.5 text-xs ${DECISION_COLORS[rule.decision]}`}
                              >
                                {rule.decision}
                              </span>
                            </td>
                            <td class="py-1 text-xs text-gray-500 dark:text-gray-400">
                              {[
                                rule.path_allow?.length
                                  ? `allow: ${rule.path_allow.join(", ")}`
                                  : "",
                                rule.path_deny?.length ? `deny: ${rule.path_deny.join(", ")}` : "",
                                rule.command_allow?.length
                                  ? `cmd-allow: ${rule.command_allow.join(", ")}`
                                  : "",
                                rule.command_deny?.length
                                  ? `cmd-deny: ${rule.command_deny.join(", ")}`
                                  : "",
                              ]
                                .filter(Boolean)
                                .join(" | ")}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </div>
            </Show>

            {/* Resource Limits */}
            <Show when={p().resource_limits}>
              {(rl) => (
                <div class="mb-4">
                  <h5 class="mb-1 text-sm font-medium text-gray-500 dark:text-gray-400">
                    {t("policy.resourceLimits")}
                  </h5>
                  <div class="flex flex-wrap gap-3 text-sm">
                    <Show when={rl().memory_mb}>
                      <span>
                        {t("policy.memory")} {rl().memory_mb}MB
                      </span>
                    </Show>
                    <Show when={rl().cpu_quota}>
                      <span>
                        {t("policy.cpu")} {rl().cpu_quota}
                      </span>
                    </Show>
                    <Show when={rl().pids_limit}>
                      <span>
                        {t("policy.pids")} {rl().pids_limit}
                      </span>
                    </Show>
                    <Show when={rl().storage_gb}>
                      <span>
                        {t("policy.storage")} {rl().storage_gb}GB
                      </span>
                    </Show>
                    <Show when={rl().network_mode}>
                      <span>
                        {t("policy.network")} {rl().network_mode}
                      </span>
                    </Show>
                  </div>
                </div>
              )}
            </Show>

            {/* Test Evaluation */}
            <div class="mb-4 rounded border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-700">
              <h5 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
                {t("policy.testEval")}
              </h5>
              <div class="flex flex-wrap gap-2">
                <input
                  class="rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("policy.toolPlaceholder")}
                  value={evalTool()}
                  onInput={(e) => setEvalTool(e.currentTarget.value)}
                  aria-label="Tool name for evaluation"
                />
                <input
                  class="rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("policy.commandPlaceholder")}
                  value={evalCommand()}
                  onInput={(e) => setEvalCommand(e.currentTarget.value)}
                  aria-label="Command for evaluation"
                />
                <input
                  class="rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("policy.pathPlaceholder")}
                  value={evalPath()}
                  onInput={(e) => setEvalPath(e.currentTarget.value)}
                  aria-label="Path for evaluation"
                />
                <button
                  class="rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                  onClick={handleEvaluate}
                  disabled={evaluating() || !evalTool()}
                >
                  {evaluating() ? "..." : t("policy.evaluate")}
                </button>
                <Show when={evalResult()}>
                  {(decision) => (
                    <span
                      class={`rounded px-2 py-1 text-sm font-medium ${DECISION_COLORS[decision()]}`}
                    >
                      {decision()}
                    </span>
                  )}
                </Show>
              </div>
            </div>

            {/* Clone button */}
            <div class="flex justify-end">
              <button
                class="rounded bg-gray-100 px-3 py-1.5 text-sm hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                onClick={handleClone}
              >
                {t("policy.cloneEdit")}
              </button>
            </div>
          </div>
        )}
      </Show>

      {/* Editor View */}
      <Show when={view() === "editor"}>
        <div class="space-y-4">
          {/* Name & Description */}
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label
                for="policy-name"
                class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                {t("policy.editor.name")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="policy-name"
                class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
                value={editProfile().name}
                onInput={(e) => updateEditField("name", e.currentTarget.value)}
                placeholder={t("policy.editor.namePlaceholder")}
                aria-required="true"
              />
            </div>
            <div>
              <label
                for="policy-mode"
                class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                {t("policy.editor.mode")}
              </label>
              <select
                id="policy-mode"
                class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
                value={editProfile().mode}
                onChange={(e) => updateEditField("mode", e.currentTarget.value as PermissionMode)}
              >
                <For each={MODES()}>{(m) => <option value={m.value}>{m.label}</option>}</For>
              </select>
            </div>
          </div>
          <div>
            <label
              for="policy-description"
              class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              {t("policy.editor.description")}
            </label>
            <input
              id="policy-description"
              class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
              value={editProfile().description || ""}
              onInput={(e) => updateEditField("description", e.currentTarget.value)}
              placeholder={t("policy.editor.descriptionPlaceholder")}
            />
          </div>

          {/* Quality Gate */}
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("policy.qualityGate")}
            </label>
            <div class="flex gap-4 text-sm">
              <label class="flex items-center gap-1">
                <input
                  type="checkbox"
                  checked={editProfile().quality_gate.require_tests_pass}
                  onChange={(e) => updateQualityGate("require_tests_pass", e.currentTarget.checked)}
                />
                {t("policy.tests")}
              </label>
              <label class="flex items-center gap-1">
                <input
                  type="checkbox"
                  checked={editProfile().quality_gate.require_lint_pass}
                  onChange={(e) => updateQualityGate("require_lint_pass", e.currentTarget.checked)}
                />
                {t("policy.lint")}
              </label>
              <label class="flex items-center gap-1">
                <input
                  type="checkbox"
                  checked={editProfile().quality_gate.rollback_on_gate_fail}
                  onChange={(e) =>
                    updateQualityGate("rollback_on_gate_fail", e.currentTarget.checked)
                  }
                />
                {t("policy.rollback")}
              </label>
            </div>
          </div>

          {/* Termination */}
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("policy.editor.termination")}
            </label>
            <div class="grid grid-cols-3 gap-3">
              <div>
                <label class="text-xs text-gray-400 dark:text-gray-500">
                  {t("policy.editor.maxSteps")}
                </label>
                <input
                  type="number"
                  class="w-full rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  value={editProfile().termination.max_steps ?? ""}
                  onInput={(e) =>
                    updateTermination("max_steps", parseInt(e.currentTarget.value) || undefined)
                  }
                />
              </div>
              <div>
                <label class="text-xs text-gray-400 dark:text-gray-500">
                  {t("policy.editor.timeoutS")}
                </label>
                <input
                  type="number"
                  class="w-full rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  value={editProfile().termination.timeout_seconds ?? ""}
                  onInput={(e) =>
                    updateTermination(
                      "timeout_seconds",
                      parseInt(e.currentTarget.value) || undefined,
                    )
                  }
                />
              </div>
              <div>
                <label class="text-xs text-gray-400 dark:text-gray-500">
                  {t("policy.editor.maxCost")}
                </label>
                <input
                  type="number"
                  step="0.01"
                  class="w-full rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                  value={editProfile().termination.max_cost ?? ""}
                  onInput={(e) =>
                    updateTermination("max_cost", parseFloat(e.currentTarget.value) || undefined)
                  }
                />
              </div>
            </div>
          </div>

          {/* Rules */}
          <div>
            <div class="mb-1 flex items-center justify-between">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {t("policy.rules")}
              </label>
              <button
                class="rounded bg-gray-100 px-2 py-0.5 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                onClick={addRule}
              >
                {t("policy.editor.addRule")}
              </button>
            </div>
            <div class="space-y-2">
              <For each={editProfile().rules}>
                {(rule, i) => (
                  <div class="flex flex-wrap items-start gap-2 rounded border border-gray-200 bg-gray-50 p-2 dark:border-gray-700 dark:bg-gray-700">
                    <input
                      class="w-20 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.toolPlaceholder")}
                      value={rule.specifier.tool}
                      onInput={(e) => updateRule(i(), "tool", e.currentTarget.value)}
                    />
                    <input
                      class="w-24 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.subPatternPlaceholder")}
                      value={rule.specifier.sub_pattern || ""}
                      onInput={(e) => updateRule(i(), "sub_pattern", e.currentTarget.value)}
                    />
                    <select
                      class="rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      value={rule.decision}
                      onChange={(e) => updateRule(i(), "decision", e.currentTarget.value)}
                    >
                      <option value="allow">{t("policy.decision.allow")}</option>
                      <option value="deny">{t("policy.decision.deny")}</option>
                      <option value="ask">{t("policy.decision.ask")}</option>
                    </select>
                    <input
                      class="w-28 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.pathAllowPlaceholder")}
                      value={rule.path_allow?.join(", ") || ""}
                      onInput={(e) => updateRule(i(), "path_allow", e.currentTarget.value)}
                    />
                    <input
                      class="w-28 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.pathDenyPlaceholder")}
                      value={rule.path_deny?.join(", ") || ""}
                      onInput={(e) => updateRule(i(), "path_deny", e.currentTarget.value)}
                    />
                    <input
                      class="w-28 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.cmdAllowPlaceholder")}
                      value={rule.command_allow?.join(", ") || ""}
                      onInput={(e) => updateRule(i(), "command_allow", e.currentTarget.value)}
                    />
                    <input
                      class="w-28 rounded border border-gray-300 px-1.5 py-1 text-xs dark:border-gray-600 dark:bg-gray-600"
                      placeholder={t("policy.editor.cmdDenyPlaceholder")}
                      value={rule.command_deny?.join(", ") || ""}
                      onInput={(e) => updateRule(i(), "command_deny", e.currentTarget.value)}
                    />
                    <button
                      type="button"
                      class="rounded px-1.5 py-1 text-xs text-red-500 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                      onClick={() => removeRule(i())}
                      aria-label={t("policy.editor.removeRuleAria", { n: String(i() + 1) })}
                    >
                      X
                    </button>
                  </div>
                )}
              </For>
            </div>
          </div>

          {/* Actions */}
          <div class="flex justify-end gap-2">
            <button
              class="rounded bg-gray-100 px-4 py-1.5 text-sm hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
              onClick={() => setView("list")}
            >
              {t("common.cancel")}
            </button>
            <button
              class="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
              onClick={handleSave}
              disabled={saving() || !editProfile().name}
            >
              {saving() ? t("common.saving") : t("common.save")}
            </button>
          </div>
        </div>
      </Show>
    </div>
  );
}
