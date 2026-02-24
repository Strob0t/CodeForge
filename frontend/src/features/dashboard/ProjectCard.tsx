import { A } from "@solidjs/router";
import { Show } from "solid-js";

import type { Project } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Button, Card } from "~/ui";

interface ProjectCardProps {
  project: Project;
  onDelete: (id: string) => void;
  onEdit: (id: string) => void;
  onDetectStack?: (id: string) => void;
  detecting?: boolean;
}

export default function ProjectCard(props: ProjectCardProps) {
  const { t, fmt } = useI18n();
  return (
    <Card class="transition-shadow hover:shadow-md">
      <Card.Body>
        <div class="flex items-start justify-between">
          <div>
            <h3 class="text-lg font-semibold text-cf-text-primary">
              <A href={`/projects/${props.project.id}`} class="hover:text-cf-accent">
                {props.project.name}
              </A>
            </h3>
            {props.project.description && (
              <p class="mt-1 text-sm text-cf-text-muted">{props.project.description}</p>
            )}
          </div>

          <div class="flex items-center gap-2">
            <Show when={props.onDetectStack && props.project.workspace_path}>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => props.onDetectStack?.(props.project.id)}
                disabled={props.detecting}
                loading={props.detecting}
              >
                {props.detecting ? t("dashboard.detect.detecting") : t("dashboard.detect.button")}
              </Button>
            </Show>

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

        <div class="mt-3 flex flex-wrap gap-3 text-xs text-cf-text-muted">
          {props.project.provider && <Badge>{props.project.provider}</Badge>}
          {props.project.repo_url && (
            <span class="truncate" title={props.project.repo_url}>
              {props.project.repo_url}
            </span>
          )}
          <span>{t("project.created", { date: fmt.date(props.project.created_at) })}</span>
        </div>
      </Card.Body>
    </Card>
  );
}
