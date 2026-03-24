import { createResource, createSignal } from "solid-js";

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
import { useConfirm } from "~/components/ConfirmProvider";
import { useAsyncAction } from "~/hooks/useAsyncAction";
import { useI18n } from "~/i18n";
import { getErrorMessage } from "~/utils/getErrorMessage";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type View = "list" | "detail" | "editor" | "preview";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function usePolicyPanel(onError: (msg: string) => void) {
  const { t } = useI18n();
  const { confirm } = useConfirm();

  // ---- Core resources ----
  const [view, setView] = createSignal<View>("list");
  const [selectedName, setSelectedName] = createSignal<string | null>(null);
  const [profiles, { refetch: refetchProfiles }] = createResource(() => api.policies.list());
  const [selectedProfile, { refetch: refetchProfile }] = createResource(
    () => selectedName(),
    (name) => (name ? api.policies.get(name) : null),
  );

  // ---- Mode options ----
  const MODES = (): { value: PermissionMode; label: string }[] => [
    { value: "default", label: t("policy.mode.default") },
    { value: "acceptEdits", label: t("policy.mode.acceptEdits") },
    { value: "plan", label: t("policy.mode.plan") },
    { value: "delegate", label: t("policy.mode.delegate") },
  ];

  // ---- Editor state ----
  const [editProfile, setEditProfile] = createSignal<PolicyProfile>(emptyProfile());

  // ---- Evaluate tester state ----
  const [evalTool, setEvalTool] = createSignal("");
  const [evalCommand, setEvalCommand] = createSignal("");
  const [evalPath, setEvalPath] = createSignal("");
  const [evalResult, setEvalResult] = createSignal<EvaluationResult | null>(null);
  const [evaluating, setEvaluating] = createSignal(false);

  // ---- Preview state ----
  const [previewPolicy, setPreviewPolicy] = createSignal<string>("");
  const [previewTool, setPreviewTool] = createSignal("");
  const [previewCommand, setPreviewCommand] = createSignal("");
  const [previewPath, setPreviewPath] = createSignal("");
  const [previewResult, setPreviewResult] = createSignal<EvaluationResult | null>(null);
  const [previewing, setPreviewing] = createSignal(false);

  // ---- Handlers ----

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

  const { run: handleSave, loading: saving } = useAsyncAction(
    async () => {
      const profile = editProfile();
      if (!profile.name) {
        onError(t("policy.toast.nameRequired"));
        return;
      }
      onError("");
      await api.policies.create(profile);
      refetchProfiles();
      setSelectedName(profile.name);
      setView("detail");
      refetchProfile();
    },
    {
      onError: (err) => onError(getErrorMessage(err, t("policy.toast.saveFailed"))),
    },
  );

  const handleDelete = async (name: string) => {
    onError("");
    const ok = await confirm({
      title: t("common.delete"),
      message: t("policy.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.policies.delete(name);
      refetchProfiles();
      if (selectedName() === name) {
        setSelectedName(null);
        setView("list");
      }
    } catch (e) {
      onError(e instanceof Error ? e.message : t("policy.toast.deleteFailed"));
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
      onError(e instanceof Error ? e.message : t("policy.toast.evalFailed"));
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
      onError(e instanceof Error ? e.message : t("policy.toast.evalFailed"));
    } finally {
      setPreviewing(false);
    }
  };

  // ---- Editor field updaters ----

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

  return {
    // View state
    view,
    setView,
    selectedName,
    setSelectedName,

    // Resources
    profiles,
    selectedProfile,

    // Mode options
    MODES,

    // Editor state
    editProfile,
    setEditProfile,

    // Evaluate tester state
    evalTool,
    setEvalTool,
    evalCommand,
    setEvalCommand,
    evalPath,
    setEvalPath,
    evalResult,
    evaluating,

    // Preview state
    previewPolicy,
    setPreviewPolicy,
    previewTool,
    setPreviewTool,
    previewCommand,
    setPreviewCommand,
    previewPath,
    setPreviewPath,
    previewResult,
    setPreviewResult,
    previewing,

    // Handlers
    handleSelect,
    handleNewPolicy,
    handleClone,
    handleSave,
    saving,
    handleDelete,
    handleEvaluate,
    handlePreview,

    // Editor field updaters
    updateEditField,
    updateTermination,
    updateQualityGate,
    addRule,
    removeRule,
    updateRule,
  };
}
