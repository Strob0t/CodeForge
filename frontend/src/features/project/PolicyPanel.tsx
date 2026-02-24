import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  EvaluationResult,
  PermissionMode,
  PermissionRule,
  PolicyDecision,
  PolicyProfile,
  PolicyQualityGate,
  PolicyToolCall,
  TerminationCondition,
} from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Checkbox, FormField, Input, Select } from "~/ui";

interface PolicyPanelProps {
  projectId: string;
  onError: (msg: string) => void;
}

type View = "list" | "detail" | "editor" | "preview";

const PRESET_NAMES = new Set([
  "plan-readonly",
  "headless-safe-sandbox",
  "headless-permissive-sandbox",
  "trusted-mount-autonomous",
]);

function decisionVariant(decision: PolicyDecision): "success" | "danger" | "warning" {
  switch (decision) {
    case "allow":
      return "success";
    case "deny":
      return "danger";
    case "ask":
      return "warning";
  }
}

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
  const [evalResult, setEvalResult] = createSignal<EvaluationResult | null>(null);
  const [evaluating, setEvaluating] = createSignal(false);

  // Preview state (standalone from list view)
  const [previewPolicy, setPreviewPolicy] = createSignal<string>("");
  const [previewTool, setPreviewTool] = createSignal("");
  const [previewCommand, setPreviewCommand] = createSignal("");
  const [previewPath, setPreviewPath] = createSignal("");
  const [previewResult, setPreviewResult] = createSignal<EvaluationResult | null>(null);
  const [previewing, setPreviewing] = createSignal(false);

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
      setEvalResult(res);
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("policy.toast.evalFailed"));
    } finally {
      setEvaluating(false);
    }
  };

  const handlePreview = async () => {
    const name = previewPolicy();
    if (!name || !previewTool()) return;
    setPreviewing(true);
    setPreviewResult(null);
    try {
      const call: PolicyToolCall = {
        tool: previewTool(),
        command: previewCommand() || undefined,
        path: previewPath() || undefined,
      };
      const res = await api.policies.evaluate(name, call);
      setPreviewResult(res);
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("policy.toast.evalFailed"));
    } finally {
      setPreviewing(false);
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
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("policy.title")}</h3>
          <div class="flex gap-2">
            <Show when={view() !== "list"}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setView("list");
                  setSelectedName(null);
                }}
              >
                {t("policy.backToList")}
              </Button>
            </Show>
            <Show when={view() === "list"}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setPreviewResult(null);
                  setView("preview");
                }}
              >
                {t("policy.preview.title")}
              </Button>
              <Button variant="primary" size="sm" onClick={handleNewPolicy}>
                {t("policy.newPolicy")}
              </Button>
            </Show>
          </div>
        </div>
      </Card.Header>

      <Card.Body>
        {/* List View */}
        <Show when={view() === "list"}>
          <Show when={profiles.loading}>
            <p class="text-sm text-cf-text-muted">{t("common.loading")}</p>
          </Show>
          <Show when={!profiles.loading && profiles()}>
            <div class="space-y-1">
              <For each={profiles()?.profiles ?? []}>
                {(name) => (
                  <div class="flex items-center justify-between rounded-cf-sm px-3 py-2 hover:bg-cf-bg-surface-alt">
                    <button
                      class="flex items-center gap-2 text-sm font-medium text-cf-text-primary"
                      onClick={() => handleSelect(name)}
                    >
                      <span>{name}</span>
                      <Badge variant={PRESET_NAMES.has(name) ? "info" : "default"}>
                        {PRESET_NAMES.has(name) ? t("policy.preset") : t("policy.custom")}
                      </Badge>
                    </button>
                    <Show when={!PRESET_NAMES.has(name)}>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(name)}
                        aria-label={t("policy.deleteAria", { name })}
                        class="text-red-500 dark:text-red-400"
                      >
                        {t("common.delete")}
                      </Button>
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
                  <p class="mt-1 text-sm text-cf-text-tertiary">{p().description}</p>
                </Show>
              </div>

              {/* Summary */}
              <div class="mb-4 grid grid-cols-3 gap-3 text-sm">
                <div>
                  <span class="text-cf-text-tertiary">{t("policy.mode")}</span>{" "}
                  <span class="font-medium">{p().mode}</span>
                </div>
                <Show when={p().termination.max_steps}>
                  <div>
                    <span class="text-cf-text-tertiary">{t("policy.steps")}</span>{" "}
                    <span class="font-medium">{p().termination.max_steps}</span>
                  </div>
                </Show>
                <Show when={p().termination.timeout_seconds}>
                  <div>
                    <span class="text-cf-text-tertiary">{t("policy.timeout")}</span>{" "}
                    <span class="font-medium">{p().termination.timeout_seconds}s</span>
                  </div>
                </Show>
                <Show when={p().termination.max_cost}>
                  <div>
                    <span class="text-cf-text-tertiary">{t("policy.cost")}</span>{" "}
                    <span class="font-medium">${p().termination.max_cost}</span>
                  </div>
                </Show>
                <Show when={p().termination.stall_detection}>
                  <div>
                    <span class="text-cf-text-tertiary">{t("policy.stall")}</span>{" "}
                    <span class="font-medium">{p().termination.stall_threshold}</span>
                  </div>
                </Show>
              </div>

              {/* Quality Gate */}
              <div class="mb-4">
                <h5 class="mb-1 text-sm font-medium text-cf-text-tertiary">
                  {t("policy.qualityGate")}
                </h5>
                <div class="flex gap-3 text-sm">
                  <span
                    class={
                      p().quality_gate.require_tests_pass ? "text-green-600" : "text-cf-text-muted"
                    }
                  >
                    {p().quality_gate.require_tests_pass ? "\u2713" : "\u2717"} {t("policy.tests")}
                  </span>
                  <span
                    class={
                      p().quality_gate.require_lint_pass ? "text-green-600" : "text-cf-text-muted"
                    }
                  >
                    {p().quality_gate.require_lint_pass ? "\u2713" : "\u2717"} {t("policy.lint")}
                  </span>
                  <span
                    class={
                      p().quality_gate.rollback_on_gate_fail
                        ? "text-green-600"
                        : "text-cf-text-muted"
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
                  <h5 class="mb-1 text-sm font-medium text-cf-text-tertiary">
                    {t("policy.rules")}
                  </h5>
                  <div class="overflow-x-auto">
                    <table class="w-full text-sm">
                      <thead>
                        <tr class="border-b border-cf-border text-left text-xs text-cf-text-tertiary">
                          <th class="pb-1 pr-3">{t("policy.table.tool")}</th>
                          <th class="pb-1 pr-3">{t("policy.table.pattern")}</th>
                          <th class="pb-1 pr-3">{t("policy.table.decision")}</th>
                          <th class="pb-1">{t("policy.table.constraints")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        <For each={p().rules}>
                          {(rule) => (
                            <tr class="border-b border-cf-border-subtle">
                              <td class="py-1 pr-3 font-mono">{rule.specifier.tool}</td>
                              <td class="py-1 pr-3 font-mono text-xs">
                                {rule.specifier.sub_pattern || ""}
                              </td>
                              <td class="py-1 pr-3">
                                <Badge variant={decisionVariant(rule.decision)}>
                                  {rule.decision}
                                </Badge>
                              </td>
                              <td class="py-1 text-xs text-cf-text-tertiary">
                                {[
                                  rule.path_allow?.length
                                    ? `allow: ${rule.path_allow.join(", ")}`
                                    : "",
                                  rule.path_deny?.length
                                    ? `deny: ${rule.path_deny.join(", ")}`
                                    : "",
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
                    <h5 class="mb-1 text-sm font-medium text-cf-text-tertiary">
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
              <div class="mb-4 rounded-cf-sm border border-cf-border bg-cf-bg-inset p-3">
                <h5 class="mb-2 text-sm font-medium text-cf-text-tertiary">
                  {t("policy.testEval")}
                </h5>
                <div class="flex flex-wrap gap-2">
                  <Input
                    placeholder={t("policy.toolPlaceholder")}
                    value={evalTool()}
                    onInput={(e) => setEvalTool(e.currentTarget.value)}
                    aria-label="Tool name for evaluation"
                  />
                  <Input
                    placeholder={t("policy.commandPlaceholder")}
                    value={evalCommand()}
                    onInput={(e) => setEvalCommand(e.currentTarget.value)}
                    aria-label="Command for evaluation"
                  />
                  <Input
                    placeholder={t("policy.pathPlaceholder")}
                    value={evalPath()}
                    onInput={(e) => setEvalPath(e.currentTarget.value)}
                    aria-label="Path for evaluation"
                  />
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleEvaluate}
                    disabled={evaluating() || !evalTool()}
                    loading={evaluating()}
                  >
                    {t("policy.evaluate")}
                  </Button>
                </div>
                <Show when={evalResult()}>
                  {(result) => (
                    <div class="mt-3 space-y-1 text-sm">
                      <div class="flex items-center gap-2">
                        <span class="text-cf-text-tertiary">
                          {t("policy.preview.result.decision")}
                        </span>
                        <Badge variant={decisionVariant(result().decision)}>
                          {result().decision}
                        </Badge>
                      </div>
                      <div>
                        <span class="text-cf-text-tertiary">
                          {t("policy.preview.result.scope")}
                        </span>{" "}
                        <span class="font-mono text-xs">{result().scope}</span>
                      </div>
                      <div>
                        <span class="text-cf-text-tertiary">
                          {t("policy.preview.result.matchedRule")}
                        </span>{" "}
                        <span class="font-mono text-xs">
                          {result().matched_rule || t("policy.preview.result.noRuleMatched")}
                        </span>
                      </div>
                      <div>
                        <span class="text-cf-text-tertiary">
                          {t("policy.preview.result.reason")}
                        </span>{" "}
                        <span class="text-xs">{result().reason}</span>
                      </div>
                    </div>
                  )}
                </Show>
              </div>

              {/* Clone button */}
              <div class="flex justify-end">
                <Button variant="secondary" size="sm" onClick={handleClone}>
                  {t("policy.cloneEdit")}
                </Button>
              </div>
            </div>
          )}
        </Show>

        {/* Preview View */}
        <Show when={view() === "preview"}>
          <div class="space-y-4">
            <p class="text-sm text-cf-text-tertiary">{t("policy.preview.description")}</p>

            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.title")} for="preview-policy">
                <Select
                  id="preview-policy"
                  value={previewPolicy()}
                  onChange={(e) => {
                    setPreviewPolicy(e.currentTarget.value);
                    setPreviewResult(null);
                  }}
                >
                  <option value="">{t("policy.preview.selectPolicy")}</option>
                  <For each={profiles()?.profiles ?? []}>
                    {(name) => <option value={name}>{name}</option>}
                  </For>
                </Select>
              </FormField>
              <FormField label={t("policy.table.tool")} for="preview-tool">
                <Input
                  id="preview-tool"
                  placeholder={t("policy.toolPlaceholder")}
                  value={previewTool()}
                  onInput={(e) => setPreviewTool(e.currentTarget.value)}
                />
              </FormField>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.commandPlaceholder")} for="preview-command">
                <Input
                  id="preview-command"
                  placeholder={t("policy.commandPlaceholder")}
                  value={previewCommand()}
                  onInput={(e) => setPreviewCommand(e.currentTarget.value)}
                />
              </FormField>
              <FormField label={t("policy.pathPlaceholder")} for="preview-path">
                <Input
                  id="preview-path"
                  placeholder={t("policy.pathPlaceholder")}
                  value={previewPath()}
                  onInput={(e) => setPreviewPath(e.currentTarget.value)}
                />
              </FormField>
            </div>

            <Button
              variant="primary"
              size="sm"
              onClick={handlePreview}
              disabled={previewing() || !previewPolicy() || !previewTool()}
              loading={previewing()}
            >
              {t("policy.evaluate")}
            </Button>

            <Show when={previewResult()}>
              {(result) => (
                <div class="rounded-cf-md border border-cf-border bg-cf-bg-surface p-4">
                  <div class="mb-3 flex items-center gap-3">
                    <Badge variant={decisionVariant(result().decision)}>
                      {result().decision.toUpperCase()}
                    </Badge>
                    <span class="text-sm text-cf-text-tertiary">{result().profile}</span>
                  </div>
                  <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-sm">
                    <dt class="text-cf-text-tertiary">{t("policy.preview.result.scope")}</dt>
                    <dd class="font-mono text-xs">{result().scope}</dd>

                    <dt class="text-cf-text-tertiary">{t("policy.preview.result.matchedRule")}</dt>
                    <dd class="font-mono text-xs">
                      {result().matched_rule || t("policy.preview.result.noRuleMatched")}
                    </dd>

                    <dt class="text-cf-text-tertiary">{t("policy.preview.result.reason")}</dt>
                    <dd class="text-xs">{result().reason}</dd>
                  </dl>
                </div>
              )}
            </Show>
          </div>
        </Show>

        {/* Editor View */}
        <Show when={view() === "editor"}>
          <div class="space-y-4">
            {/* Name & Description */}
            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.editor.name")} for="policy-name" required>
                <Input
                  id="policy-name"
                  value={editProfile().name}
                  onInput={(e) => updateEditField("name", e.currentTarget.value)}
                  placeholder={t("policy.editor.namePlaceholder")}
                  aria-required="true"
                />
              </FormField>
              <FormField label={t("policy.editor.mode")} for="policy-mode">
                <Select
                  id="policy-mode"
                  value={editProfile().mode}
                  onChange={(e) => updateEditField("mode", e.currentTarget.value as PermissionMode)}
                >
                  <For each={MODES()}>{(m) => <option value={m.value}>{m.label}</option>}</For>
                </Select>
              </FormField>
            </div>
            <FormField label={t("policy.editor.description")} for="policy-description">
              <Input
                id="policy-description"
                value={editProfile().description || ""}
                onInput={(e) => updateEditField("description", e.currentTarget.value)}
                placeholder={t("policy.editor.descriptionPlaceholder")}
              />
            </FormField>

            {/* Quality Gate */}
            <div>
              <label class="mb-1 block text-xs font-medium text-cf-text-tertiary">
                {t("policy.qualityGate")}
              </label>
              <div class="flex gap-4 text-sm">
                <Checkbox
                  checked={editProfile().quality_gate.require_tests_pass}
                  onChange={(e) => updateQualityGate("require_tests_pass", e.currentTarget.checked)}
                  label={t("policy.tests")}
                />
                <Checkbox
                  checked={editProfile().quality_gate.require_lint_pass}
                  onChange={(e) => updateQualityGate("require_lint_pass", e.currentTarget.checked)}
                  label={t("policy.lint")}
                />
                <Checkbox
                  checked={editProfile().quality_gate.rollback_on_gate_fail}
                  onChange={(e) =>
                    updateQualityGate("rollback_on_gate_fail", e.currentTarget.checked)
                  }
                  label={t("policy.rollback")}
                />
              </div>
            </div>

            {/* Termination */}
            <div>
              <label class="mb-1 block text-xs font-medium text-cf-text-tertiary">
                {t("policy.editor.termination")}
              </label>
              <div class="grid grid-cols-3 gap-3">
                <FormField label={t("policy.editor.maxSteps")}>
                  <Input
                    type="number"
                    value={editProfile().termination.max_steps ?? ""}
                    onInput={(e) =>
                      updateTermination("max_steps", parseInt(e.currentTarget.value) || undefined)
                    }
                  />
                </FormField>
                <FormField label={t("policy.editor.timeoutS")}>
                  <Input
                    type="number"
                    value={editProfile().termination.timeout_seconds ?? ""}
                    onInput={(e) =>
                      updateTermination(
                        "timeout_seconds",
                        parseInt(e.currentTarget.value) || undefined,
                      )
                    }
                  />
                </FormField>
                <FormField label={t("policy.editor.maxCost")}>
                  <Input
                    type="number"
                    step="0.01"
                    value={editProfile().termination.max_cost ?? ""}
                    onInput={(e) =>
                      updateTermination("max_cost", parseFloat(e.currentTarget.value) || undefined)
                    }
                  />
                </FormField>
              </div>
            </div>

            {/* Rules */}
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="text-xs font-medium text-cf-text-tertiary">{t("policy.rules")}</label>
                <Button variant="secondary" size="sm" onClick={addRule}>
                  {t("policy.editor.addRule")}
                </Button>
              </div>
              <div class="space-y-2">
                <For each={editProfile().rules}>
                  {(rule, i) => (
                    <div class="flex flex-wrap items-start gap-2 rounded-cf-sm border border-cf-border bg-cf-bg-inset p-2">
                      <Input
                        class="w-20"
                        placeholder={t("policy.editor.toolPlaceholder")}
                        value={rule.specifier.tool}
                        onInput={(e) => updateRule(i(), "tool", e.currentTarget.value)}
                      />
                      <Input
                        class="w-24"
                        placeholder={t("policy.editor.subPatternPlaceholder")}
                        value={rule.specifier.sub_pattern || ""}
                        onInput={(e) => updateRule(i(), "sub_pattern", e.currentTarget.value)}
                      />
                      <Select
                        value={rule.decision}
                        onChange={(e) => updateRule(i(), "decision", e.currentTarget.value)}
                      >
                        <option value="allow">{t("policy.decision.allow")}</option>
                        <option value="deny">{t("policy.decision.deny")}</option>
                        <option value="ask">{t("policy.decision.ask")}</option>
                      </Select>
                      <Input
                        class="w-28"
                        placeholder={t("policy.editor.pathAllowPlaceholder")}
                        value={rule.path_allow?.join(", ") || ""}
                        onInput={(e) => updateRule(i(), "path_allow", e.currentTarget.value)}
                      />
                      <Input
                        class="w-28"
                        placeholder={t("policy.editor.pathDenyPlaceholder")}
                        value={rule.path_deny?.join(", ") || ""}
                        onInput={(e) => updateRule(i(), "path_deny", e.currentTarget.value)}
                      />
                      <Input
                        class="w-28"
                        placeholder={t("policy.editor.cmdAllowPlaceholder")}
                        value={rule.command_allow?.join(", ") || ""}
                        onInput={(e) => updateRule(i(), "command_allow", e.currentTarget.value)}
                      />
                      <Input
                        class="w-28"
                        placeholder={t("policy.editor.cmdDenyPlaceholder")}
                        value={rule.command_deny?.join(", ") || ""}
                        onInput={(e) => updateRule(i(), "command_deny", e.currentTarget.value)}
                      />
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => removeRule(i())}
                        aria-label={t("policy.editor.removeRuleAria", { n: String(i() + 1) })}
                        class="text-red-500 dark:text-red-400"
                      >
                        X
                      </Button>
                    </div>
                  )}
                </For>
              </div>
            </div>

            {/* Actions */}
            <div class="flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => setView("list")}>
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={handleSave}
                disabled={saving() || !editProfile().name}
                loading={saving()}
              >
                {t("common.save")}
              </Button>
            </div>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}
