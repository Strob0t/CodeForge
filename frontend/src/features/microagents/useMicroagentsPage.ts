import { createResource, createSignal } from "solid-js";

import { api } from "~/api/client";
import type {
  CreateMicroagentRequest,
  CreateSkillRequest,
  Microagent,
  MicroagentType,
  Skill,
  UpdateMicroagentRequest,
  UpdateSkillRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type MicroagentsTab = "microagents" | "skills";

// ---------------------------------------------------------------------------
// Form defaults
// ---------------------------------------------------------------------------

interface MicroagentFormState {
  name: string;
  type: MicroagentType;
  trigger_pattern: string;
  description: string;
  prompt: string;
  enabled: boolean;
}

const MA_FORM_DEFAULTS: MicroagentFormState = {
  name: "",
  type: "knowledge",
  trigger_pattern: "",
  description: "",
  prompt: "",
  enabled: true,
};

interface SkillFormState {
  name: string;
  type: string;
  description: string;
  language: string;
  content: string;
  tags: string;
  status: string;
}

const SK_FORM_DEFAULTS: SkillFormState = {
  name: "",
  type: "workflow",
  description: "",
  language: "",
  content: "",
  tags: "",
  status: "draft",
};

// ---------------------------------------------------------------------------
// Page-level hook
// ---------------------------------------------------------------------------

export function useMicroagentsPage() {
  const [activeTab, setActiveTab] = createSignal<MicroagentsTab>("microagents");
  const [selectedProjectId, setSelectedProjectId] = createSignal("");
  const [projects] = createResource(() => api.projects.list());

  return { activeTab, setActiveTab, selectedProjectId, setSelectedProjectId, projects };
}

// ---------------------------------------------------------------------------
// Microagents tab hook
// ---------------------------------------------------------------------------

export function useMicroagentsTab(projectId: () => string) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [microagents, { refetch }] = createResource(
    () => projectId(),
    (pid) => api.microagents.list(pid),
  );

  const crud = useCRUDForm(MA_FORM_DEFAULTS, async (ma: Microagent) => {
    await api.microagents.delete(ma.id);
    toast("success", t("microagents.toast.deleted"));
    refetch();
  });

  function handleEdit(ma: Microagent): void {
    crud.startEdit(ma.id, {
      name: ma.name,
      type: ma.type,
      trigger_pattern: ma.trigger_pattern,
      description: ma.description,
      prompt: ma.prompt,
      enabled: ma.enabled,
    });
  }

  const {
    run: handleSubmit,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const name = crud.form.state.name.trim();
      if (!name) return;
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        const data: UpdateMicroagentRequest = {
          name,
          trigger_pattern: crud.form.state.trigger_pattern,
          description: crud.form.state.description,
          prompt: crud.form.state.prompt,
          enabled: crud.form.state.enabled,
        };
        await api.microagents.update(eid, data);
        toast("success", t("microagents.toast.updated"));
      } else {
        const data: CreateMicroagentRequest = {
          name,
          type: crud.form.state.type,
          trigger_pattern: crud.form.state.trigger_pattern,
          description: crud.form.state.description,
          prompt: crud.form.state.prompt,
        };
        await api.microagents.create(projectId(), data);
        toast("success", t("microagents.toast.created"));
      }
      crud.cancelForm();
      refetch();
    },
    {
      onError: (err) => {
        const msg = extractErrorMessage(err, "Failed");
        toast("error", msg);
      },
    },
  );

  return { microagents, crud, handleEdit, handleSubmit, error, clearError };
}

// ---------------------------------------------------------------------------
// Skills tab hook
// ---------------------------------------------------------------------------

function parseTags(raw: string): string[] {
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

export function useSkillsTab(projectId: () => string) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [skills, { refetch }] = createResource(
    () => projectId(),
    (pid) => api.skills.list(pid),
  );

  const crud = useCRUDForm(SK_FORM_DEFAULTS, async (sk: Skill) => {
    await api.skills.delete(sk.id);
    toast("success", t("skills.toast.deleted"));
    refetch();
  });

  // Import state
  const [showImport, setShowImport] = createSignal(false);
  const [importUrl, setImportUrl] = createSignal("");

  const {
    run: handleImport,
    loading: importing,
    error: importError,
    clearError: clearImportError,
  } = useAsyncAction(
    async () => {
      await api.skills.import({
        source_url: importUrl(),
        project_id: projectId(),
      });
      toast("success", t("skills.toast.imported"));
      setShowImport(false);
      setImportUrl("");
      refetch();
    },
    {
      onError: (err) => {
        const msg = extractErrorMessage(err, "Import failed");
        toast("error", msg);
      },
    },
  );

  function handleEdit(sk: Skill): void {
    crud.startEdit(sk.id, {
      name: sk.name,
      type: sk.type,
      description: sk.description,
      language: sk.language,
      content: sk.content,
      tags: (sk.tags ?? []).join(", "),
      status: sk.status,
    });
  }

  const {
    run: handleSubmit,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const name = crud.form.state.name.trim();
      if (!name) return;
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        const data: UpdateSkillRequest = {
          name,
          type: crud.form.state.type,
          description: crud.form.state.description,
          language: crud.form.state.language,
          content: crud.form.state.content,
          tags: parseTags(crud.form.state.tags),
          status: crud.form.state.status,
        };
        await api.skills.update(eid, data);
        toast("success", t("skills.toast.updated"));
      } else {
        const data: CreateSkillRequest = {
          name,
          type: crud.form.state.type,
          description: crud.form.state.description,
          language: crud.form.state.language,
          content: crud.form.state.content,
          tags: parseTags(crud.form.state.tags),
        };
        await api.skills.create(projectId(), data);
        toast("success", t("skills.toast.created"));
      }
      crud.cancelForm();
      refetch();
    },
    {
      onError: (err) => {
        const msg = extractErrorMessage(err, "Failed");
        toast("error", msg);
      },
    },
  );

  return {
    skills,
    crud,
    handleEdit,
    handleSubmit,
    error,
    clearError,
    showImport,
    setShowImport,
    importUrl,
    setImportUrl,
    handleImport,
    importing,
    importError,
    clearImportError,
  };
}
