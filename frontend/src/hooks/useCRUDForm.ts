import type { Accessor } from "solid-js";
import { createSignal } from "solid-js";

import { useConfirmAction } from "./useConfirmAction";
import { useFormState } from "./useFormState";

interface CRUDFormReturn<TForm extends object, TDelete> {
  /** Whether the create/edit form is visible. */
  showForm: Accessor<boolean>;
  setShowForm: (v: boolean) => void;
  /** The ID of the entity being edited, or null for create mode. */
  editingId: Accessor<string | null>;
  /** True when editing an existing entity. */
  isEditing: () => boolean;
  /** Show the form in create mode (resets form state). */
  startCreate: () => void;
  /** Show the form in edit mode with populated values. */
  startEdit: (id: string, values: Partial<TForm>) => void;
  /** Hide the form and reset all state. */
  cancelForm: () => void;
  /** Form state from useFormState (state, setState, reset, populate). */
  form: ReturnType<typeof useFormState<TForm>>;
  /** Delete confirmation from useConfirmAction (target, requestConfirm, confirm, cancel). */
  del: ReturnType<typeof useConfirmAction<TDelete>>;
}

/**
 * Composite hook for CRUD pages that combines showForm/editingId management,
 * useFormState for form data, and optionally useConfirmAction for delete confirmation.
 *
 * Usage:
 * ```ts
 * const crud = useCRUDForm(
 *   { name: "", desc: "" },
 *   async (item: Server) => { await api.delete(item.id); refetch(); }
 * );
 * // crud.startCreate()   — open blank form
 * // crud.startEdit(id, { name: "foo" }) — open form with values
 * // crud.cancelForm()    — close & reset
 * // crud.del.requestConfirm(server) — trigger delete confirmation
 * ```
 */
export function useCRUDForm<TForm extends object, TDelete = unknown>(
  defaults: TForm,
  onDelete?: (item: TDelete) => Promise<void>,
): CRUDFormReturn<TForm, TDelete> {
  const [showForm, setShowForm] = createSignal(false);
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const form = useFormState(defaults);
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  const del = useConfirmAction(onDelete ?? (async () => {}));

  const isEditing = () => editingId() !== null;

  const startCreate = () => {
    form.reset();
    setEditingId(null);
    setShowForm(true);
  };

  const startEdit = (id: string, values: Partial<TForm>) => {
    form.populate(values);
    setEditingId(id);
    setShowForm(true);
  };

  const cancelForm = () => {
    setShowForm(false);
    form.reset();
    setEditingId(null);
  };

  return {
    showForm,
    setShowForm,
    editingId,
    isEditing,
    startCreate,
    startEdit,
    cancelForm,
    form,
    del,
  };
}
