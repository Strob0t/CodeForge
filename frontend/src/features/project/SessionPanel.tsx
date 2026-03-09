import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Session } from "~/api/types";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";
import { Badge, Card } from "~/ui";

interface SessionPanelProps {
  projectId: string;
}

interface SessionTreeNode {
  session: Session;
  children: SessionTreeNode[];
}

function buildTree(sessions: Session[]): SessionTreeNode[] {
  const map = new Map<string, SessionTreeNode>();
  const roots: SessionTreeNode[] = [];

  for (const s of sessions) {
    map.set(s.id, { session: s, children: [] });
  }

  for (const s of sessions) {
    const node = map.get(s.id);
    if (!node) continue;
    const parent = s.parent_session_id ? map.get(s.parent_session_id) : undefined;
    if (parent) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }

  return roots;
}

const STATUS_VARIANT: Record<string, "success" | "info" | "warning" | "default"> = {
  active: "success",
  paused: "warning",
  completed: "default",
  forked: "info",
};

function SessionTreeNodeView(props: {
  node: SessionTreeNode;
  depth: number;
  t: ReturnType<typeof useI18n>["t"];
  fmt: ReturnType<typeof useI18n>["fmt"];
}) {
  return (
    <>
      <div
        class="flex items-center gap-2 py-2 px-3 rounded hover:bg-cf-bg-hover transition-colors"
        style={{ "padding-left": `${props.depth * 20 + 12}px` }}
      >
        <Badge variant={STATUS_VARIANT[props.node.session.status] ?? "default"} pill>
          {props.t(("session.status." + props.node.session.status) as TranslationKey)}
        </Badge>
        <span class="text-sm text-cf-text-primary font-mono truncate">
          {props.node.session.id.slice(0, 8)}
        </span>
        <Show when={props.node.session.conversation_id}>
          {(convId) => <span class="text-xs text-cf-text-muted">conv: {convId().slice(0, 8)}</span>}
        </Show>
        <span class="flex-1" />
        <span class="text-xs text-cf-text-muted">
          {props.fmt.date(props.node.session.created_at)}
        </span>
      </div>
      <For each={props.node.children}>
        {(child) => (
          <SessionTreeNodeView
            node={child}
            depth={Math.min(props.depth + 1, 3)}
            t={props.t}
            fmt={props.fmt}
          />
        )}
      </For>
    </>
  );
}

export default function SessionPanel(props: SessionPanelProps) {
  const { t, fmt } = useI18n();
  const [sessions] = createResource(
    () => props.projectId,
    (id) => api.sessions.list(id),
  );

  const tree = () => {
    const data = sessions();
    if (!data || data.length === 0) return [];
    return buildTree(data);
  };

  return (
    <Card>
      <div class="p-4">
        <h3 class="text-lg font-semibold text-cf-text-primary mb-3">{t("session.title")}</h3>
        <Show
          when={!sessions.loading && tree().length > 0}
          fallback={
            <Show
              when={!sessions.loading}
              fallback={<p class="text-sm text-cf-text-muted">Loading...</p>}
            >
              <p class="text-sm text-cf-text-muted">{t("session.empty")}</p>
            </Show>
          }
        >
          <div class="space-y-0.5">
            <For each={tree()}>
              {(node) => <SessionTreeNodeView node={node} depth={0} t={t} fmt={fmt} />}
            </For>
          </div>
        </Show>
      </div>
    </Card>
  );
}
