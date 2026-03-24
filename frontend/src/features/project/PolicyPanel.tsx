import { For, Show } from "solid-js";

import type { PermissionMode, PolicyDecision } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Checkbox, FormField, Input, Select } from "~/ui";

import { usePolicyPanel } from "./usePolicyPanel";

// ---------------------------------------------------------------------------
// Constants & helpers (render-only)
// ---------------------------------------------------------------------------

interface PolicyPanelProps {
  projectId: string;
  onError: (msg: string) => void;
}

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

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function PolicyPanel(props: PolicyPanelProps) {
  const { t } = useI18n();
  const state = usePolicyPanel(props.onError);

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("policy.title")}</h3>
          <div class="flex gap-2">
            <Show when={state.view() !== "list"}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  state.setView("list");
                  state.setSelectedName(null);
                }}
              >
                {t("policy.backToList")}
              </Button>
            </Show>
            <Show when={state.view() === "list"}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  state.setPreviewResult(null);
                  state.setView("preview");
                }}
              >
                {t("policy.preview.title")}
              </Button>
              <Button variant="primary" size="sm" onClick={state.handleNewPolicy}>
                {t("policy.newPolicy")}
              </Button>
            </Show>
          </div>
        </div>
      </Card.Header>

      <Card.Body>
        {/* List View */}
        <Show when={state.view() === "list"}>
          <Show when={state.profiles.loading}>
            <p class="text-sm text-cf-text-muted">{t("common.loading")}</p>
          </Show>
          <Show when={!state.profiles.loading && state.profiles()}>
            <div class="space-y-1">
              <For each={state.profiles()?.profiles ?? []}>
                {(name) => (
                  <div class="flex items-center justify-between rounded-cf-sm px-3 py-2 hover:bg-cf-bg-surface-alt">
                    <Button
                      variant="ghost"
                      size="sm"
                      class="flex items-center gap-2"
                      onClick={() => state.handleSelect(name)}
                    >
                      <span>{name}</span>
                      <Badge variant={PRESET_NAMES.has(name) ? "info" : "default"}>
                        {PRESET_NAMES.has(name) ? t("policy.preset") : t("policy.custom")}
                      </Badge>
                    </Button>
                    <Show when={!PRESET_NAMES.has(name)}>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => state.handleDelete(name)}
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
        <Show when={state.view() === "detail" && state.selectedProfile()}>
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
                    value={state.evalTool()}
                    onInput={(e) => state.setEvalTool(e.currentTarget.value)}
                    aria-label="Tool name for evaluation"
                  />
                  <Input
                    placeholder={t("policy.commandPlaceholder")}
                    value={state.evalCommand()}
                    onInput={(e) => state.setEvalCommand(e.currentTarget.value)}
                    aria-label="Command for evaluation"
                  />
                  <Input
                    placeholder={t("policy.pathPlaceholder")}
                    value={state.evalPath()}
                    onInput={(e) => state.setEvalPath(e.currentTarget.value)}
                    aria-label="Path for evaluation"
                  />
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={state.handleEvaluate}
                    disabled={state.evaluating() || !state.evalTool()}
                    loading={state.evaluating()}
                  >
                    {t("policy.evaluate")}
                  </Button>
                </div>
                <Show when={state.evalResult()}>
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
                <Button variant="secondary" size="sm" onClick={state.handleClone}>
                  {t("policy.cloneEdit")}
                </Button>
              </div>
            </div>
          )}
        </Show>

        {/* Preview View */}
        <Show when={state.view() === "preview"}>
          <div class="space-y-4">
            <p class="text-sm text-cf-text-tertiary">{t("policy.preview.description")}</p>

            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.title")} id="preview-policy">
                <Select
                  id="preview-policy"
                  value={state.previewPolicy()}
                  onChange={(e) => {
                    state.setPreviewPolicy(e.currentTarget.value);
                    state.setPreviewResult(null);
                  }}
                >
                  <option value="">{t("policy.preview.selectPolicy")}</option>
                  <For each={state.profiles()?.profiles ?? []}>
                    {(name) => <option value={name}>{name}</option>}
                  </For>
                </Select>
              </FormField>
              <FormField label={t("policy.table.tool")} id="preview-tool">
                <Input
                  id="preview-tool"
                  placeholder={t("policy.toolPlaceholder")}
                  value={state.previewTool()}
                  onInput={(e) => state.setPreviewTool(e.currentTarget.value)}
                />
              </FormField>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.commandPlaceholder")} id="preview-command">
                <Input
                  id="preview-command"
                  placeholder={t("policy.commandPlaceholder")}
                  value={state.previewCommand()}
                  onInput={(e) => state.setPreviewCommand(e.currentTarget.value)}
                />
              </FormField>
              <FormField label={t("policy.pathPlaceholder")} id="preview-path">
                <Input
                  id="preview-path"
                  placeholder={t("policy.pathPlaceholder")}
                  value={state.previewPath()}
                  onInput={(e) => state.setPreviewPath(e.currentTarget.value)}
                />
              </FormField>
            </div>

            <Button
              variant="primary"
              size="sm"
              onClick={state.handlePreview}
              disabled={state.previewing() || !state.previewPolicy() || !state.previewTool()}
              loading={state.previewing()}
            >
              {t("policy.evaluate")}
            </Button>

            <Show when={state.previewResult()}>
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
        <Show when={state.view() === "editor"}>
          <div class="space-y-4">
            {/* Name & Description */}
            <div class="grid grid-cols-2 gap-3">
              <FormField label={t("policy.editor.name")} id="policy-name" required>
                <Input
                  id="policy-name"
                  value={state.editProfile().name}
                  onInput={(e) => state.updateEditField("name", e.currentTarget.value)}
                  placeholder={t("policy.editor.namePlaceholder")}
                  aria-required="true"
                />
              </FormField>
              <FormField label={t("policy.editor.mode")} id="policy-mode">
                <Select
                  id="policy-mode"
                  value={state.editProfile().mode}
                  onChange={(e) =>
                    state.updateEditField("mode", e.currentTarget.value as PermissionMode)
                  }
                >
                  <For each={state.MODES()}>
                    {(m) => <option value={m.value}>{m.label}</option>}
                  </For>
                </Select>
              </FormField>
            </div>
            <FormField label={t("policy.editor.description")} id="policy-description">
              <Input
                id="policy-description"
                value={state.editProfile().description || ""}
                onInput={(e) => state.updateEditField("description", e.currentTarget.value)}
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
                  checked={state.editProfile().quality_gate.require_tests_pass}
                  onChange={(checked) => state.updateQualityGate("require_tests_pass", checked)}
                  label={t("policy.tests")}
                />
                <Checkbox
                  checked={state.editProfile().quality_gate.require_lint_pass}
                  onChange={(checked) => state.updateQualityGate("require_lint_pass", checked)}
                  label={t("policy.lint")}
                />
                <Checkbox
                  checked={state.editProfile().quality_gate.rollback_on_gate_fail}
                  onChange={(checked) => state.updateQualityGate("rollback_on_gate_fail", checked)}
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
                    value={state.editProfile().termination.max_steps ?? ""}
                    onInput={(e) =>
                      state.updateTermination(
                        "max_steps",
                        parseInt(e.currentTarget.value) || undefined,
                      )
                    }
                  />
                </FormField>
                <FormField label={t("policy.editor.timeoutS")}>
                  <Input
                    type="number"
                    value={state.editProfile().termination.timeout_seconds ?? ""}
                    onInput={(e) =>
                      state.updateTermination(
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
                    value={state.editProfile().termination.max_cost ?? ""}
                    onInput={(e) =>
                      state.updateTermination(
                        "max_cost",
                        parseFloat(e.currentTarget.value) || undefined,
                      )
                    }
                  />
                </FormField>
              </div>
            </div>

            {/* Rules */}
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="text-xs font-medium text-cf-text-tertiary">{t("policy.rules")}</label>
                <Button variant="secondary" size="sm" onClick={state.addRule}>
                  {t("policy.editor.addRule")}
                </Button>
              </div>
              <div class="space-y-2">
                <For each={state.editProfile().rules}>
                  {(rule, i) => (
                    <div class="flex flex-wrap items-start gap-2 rounded-cf-sm border border-cf-border bg-cf-bg-inset p-2">
                      <Input
                        aria-label="Tool pattern"
                        class="w-20"
                        placeholder={t("policy.editor.toolPlaceholder")}
                        value={rule.specifier.tool}
                        onInput={(e) => state.updateRule(i(), "tool", e.currentTarget.value)}
                      />
                      <Input
                        aria-label="Sub-pattern"
                        class="w-24"
                        placeholder={t("policy.editor.subPatternPlaceholder")}
                        value={rule.specifier.sub_pattern || ""}
                        onInput={(e) => state.updateRule(i(), "sub_pattern", e.currentTarget.value)}
                      />
                      <Select
                        aria-label="Action"
                        value={rule.decision}
                        onChange={(e) => state.updateRule(i(), "decision", e.currentTarget.value)}
                      >
                        <option value="allow">{t("policy.decision.allow")}</option>
                        <option value="deny">{t("policy.decision.deny")}</option>
                        <option value="ask">{t("policy.decision.ask")}</option>
                      </Select>
                      <Input
                        aria-label="Path allow pattern"
                        class="w-28"
                        placeholder={t("policy.editor.pathAllowPlaceholder")}
                        value={rule.path_allow?.join(", ") || ""}
                        onInput={(e) => state.updateRule(i(), "path_allow", e.currentTarget.value)}
                      />
                      <Input
                        aria-label="Path deny pattern"
                        class="w-28"
                        placeholder={t("policy.editor.pathDenyPlaceholder")}
                        value={rule.path_deny?.join(", ") || ""}
                        onInput={(e) => state.updateRule(i(), "path_deny", e.currentTarget.value)}
                      />
                      <Input
                        aria-label="Command allow pattern"
                        class="w-28"
                        placeholder={t("policy.editor.cmdAllowPlaceholder")}
                        value={rule.command_allow?.join(", ") || ""}
                        onInput={(e) =>
                          state.updateRule(i(), "command_allow", e.currentTarget.value)
                        }
                      />
                      <Input
                        aria-label="Command deny pattern"
                        class="w-28"
                        placeholder={t("policy.editor.cmdDenyPlaceholder")}
                        value={rule.command_deny?.join(", ") || ""}
                        onInput={(e) =>
                          state.updateRule(i(), "command_deny", e.currentTarget.value)
                        }
                      />
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => state.removeRule(i())}
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
              <Button variant="secondary" size="sm" onClick={() => state.setView("list")}>
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={state.handleSave}
                disabled={state.saving() || !state.editProfile().name}
                loading={state.saving()}
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
