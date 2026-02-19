import { A } from "@solidjs/router";

import type { Project } from "~/api/types";

interface ProjectCardProps {
  project: Project;
  onDelete: (id: string) => void;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export default function ProjectCard(props: ProjectCardProps) {
  return (
    <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5 shadow-sm dark:shadow-gray-900/30 transition-shadow hover:shadow-md dark:hover:shadow-gray-900/30">
      <div class="flex items-start justify-between">
        <div>
          <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            <A
              href={`/projects/${props.project.id}`}
              class="hover:text-blue-600 dark:hover:text-blue-400"
            >
              {props.project.name}
            </A>
          </h3>
          {props.project.description && (
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{props.project.description}</p>
          )}
        </div>

        <button
          type="button"
          class="rounded px-2 py-1 text-sm text-red-500 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 hover:text-red-700 dark:hover:text-red-400"
          onClick={() => props.onDelete(props.project.id)}
          aria-label={`Delete project ${props.project.name}`}
        >
          Delete
        </button>
      </div>

      <div class="mt-3 flex flex-wrap gap-3 text-xs text-gray-400 dark:text-gray-500">
        {props.project.provider && (
          <span class="rounded bg-gray-100 dark:bg-gray-700 px-2 py-0.5 text-gray-600 dark:text-gray-400">
            {props.project.provider}
          </span>
        )}
        {props.project.repo_url && (
          <span class="truncate" title={props.project.repo_url}>
            {props.project.repo_url}
          </span>
        )}
        <span>Created {formatDate(props.project.created_at)}</span>
      </div>
    </div>
  );
}
