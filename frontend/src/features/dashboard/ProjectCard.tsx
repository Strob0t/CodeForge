import { A, useNavigate } from "@solidjs/router";
import { Show } from "solid-js";

import type { Project, ProjectHealth } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Button, Card } from "~/ui";

import HealthDot from "./HealthDot";

interface ProjectCardProps {
  project: Project;
  health: ProjectHealth | undefined;
  onDelete: (id: string) => void;
  onEdit: (id: string) => void;
  batchMode?: boolean;
  selected?: boolean;
  onToggleSelect?: (id: string) => void;
}

export default function ProjectCard(props: ProjectCardProps) {
  const { t, fmt } = useI18n();
  const navigate = useNavigate();

  const handleCardClick = (e: MouseEvent) => {
    if ((e.target as HTMLElement).closest("button")) return;
    if ((e.target as HTMLElement).closest("a")) return;
    navigate(`/projects/${props.project.id}`);
  };

  return (
    <Card
      class={`hover:shadow-cf-md hover:border-cf-accent/30 transition-all duration-200 cursor-pointer ${props.selected ? "ring-2 ring-cf-accent" : ""}`}
      onClick={handleCardClick}
    >
      <Card.Body>
        {/* Header row: name + health dot */}
        <div class="flex items-start justify-between">
          <div class="flex items-center gap-2">
            <Show when={props.batchMode}>
              <input
                type="checkbox"
                checked={props.selected}
                onChange={() => props.onToggleSelect?.(props.project.id)}
                class="h-4 w-4 rounded border-cf-border text-cf-accent accent-cf-accent"
                aria-label={`Select ${props.project.name}`}
              />
            </Show>
            <Show when={props.health}>
              {(h) => <HealthDot score={h().score} level={h().level} factors={h().factors} />}
            </Show>
            <h3 class="text-lg font-semibold text-cf-text-primary">
              <A href={`/projects/${props.project.id}`} class="hover:text-cf-accent">
                {props.project.name}
              </A>
            </h3>
          </div>
        </div>

        {props.project.description && (
          <p class="mt-1 text-sm text-cf-text-muted line-clamp-2">{props.project.description}</p>
        )}

        {/* Stats row */}
        <Show when={props.health?.stats}>
          {(stats) => (
            <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--cf-text-secondary)]">
              <span>Runs: {stats().total_runs_7d}</span>
              <span>Success: {stats().success_rate_pct.toFixed(0)}%</span>
              <span>Cost: ${stats().total_cost_usd.toFixed(2)}</span>
              <Show when={stats().active_agents > 0}>
                <span>Agents: {stats().active_agents}</span>
              </Show>
            </div>
          )}
        </Show>

        {/* Footer: metadata + actions */}
        <div class="mt-3 flex items-center justify-between">
          <div class="flex flex-wrap gap-2 text-xs text-cf-text-muted">
            {props.project.provider && <Badge>{props.project.provider}</Badge>}
            <span>{t("project.created", { date: fmt.date(props.project.created_at) })}</span>
          </div>
          <div class="flex gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => props.onEdit(props.project.id)}
              aria-label={t("project.editAria", { name: props.project.name })}
            >
              {t("project.edit")}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="text-cf-danger-fg hover:text-cf-danger-fg"
              onClick={() => props.onDelete(props.project.id)}
              aria-label={t("project.deleteAria", { name: props.project.name })}
            >
              {t("project.delete")}
            </Button>
          </div>
        </div>
      </Card.Body>
    </Card>
  );
}
