import type { BadgeVariant } from "~/ui/primitives/Badge";

/** Look up a variant from a map, falling back to a default. */
export function getVariant(
  map: Record<string, BadgeVariant>,
  key: string,
  fallback: BadgeVariant = "default",
): BadgeVariant {
  return map[key] ?? fallback;
}

// Plan status → badge variant
export const planStatusVariant: Record<string, BadgeVariant> = {
  pending: "default",
  running: "info",
  completed: "success",
  failed: "danger",
  cancelled: "warning",
};

// Plan step status → badge variant
export const stepStatusVariant: Record<string, BadgeVariant> = {
  pending: "default",
  running: "info",
  completed: "success",
  failed: "danger",
  skipped: "default",
  cancelled: "warning",
};

// Run status → badge variant
export const runStatusVariant: Record<string, BadgeVariant> = {
  pending: "default",
  running: "info",
  completed: "success",
  failed: "danger",
  cancelled: "warning",
  timeout: "warning",
  quality_gate: "primary",
};

// Feature status → badge variant
export const featureStatusVariant: Record<string, BadgeVariant> = {
  backlog: "default",
  planned: "info",
  in_progress: "warning",
  done: "success",
  cancelled: "danger",
};

// Roadmap status → badge variant
export const roadmapStatusVariant: Record<string, BadgeVariant> = {
  draft: "default",
  active: "info",
  complete: "success",
  archived: "warning",
};

// Team status → badge variant
export const teamStatusVariant: Record<string, BadgeVariant> = {
  initializing: "default",
  active: "success",
  completed: "info",
  failed: "danger",
};

// Team role → badge variant
export const teamRoleVariant: Record<string, BadgeVariant> = {
  coder: "info",
  reviewer: "primary",
  tester: "success",
  documenter: "warning",
  planner: "danger",
};

// Agent status → badge variant
export const agentStatusVariant: Record<string, BadgeVariant> = {
  idle: "success",
  running: "info",
  error: "danger",
  stopped: "default",
};

// Generic step/node status (used in AgentFlowGraph, StepDetailPanel)
export const nodeStatusVariant: Record<string, BadgeVariant> = {
  running: "info",
  completed: "success",
  failed: "danger",
  review: "warning",
  cancelled: "warning",
};

// Benchmark run status → badge variant
export const benchmarkStatusVariant: Record<string, BadgeVariant> = {
  completed: "success",
  failed: "danger",
  pending: "warning",
  running: "warning",
};

// Severity → badge variant (activity page)
export const severityVariant: Record<string, BadgeVariant> = {
  info: "info",
  success: "success",
  warning: "warning",
  error: "danger",
};

// Knowledge base status → badge variant
export const kbStatusVariant: Record<string, BadgeVariant> = {
  pending: "warning",
  indexed: "success",
  error: "danger",
};

// Knowledge base category → badge variant
export const kbCategoryVariant: Record<string, BadgeVariant> = {
  framework: "info",
  paradigm: "primary",
  language: "success",
  security: "danger",
  custom: "default",
};

// Scope type → badge variant
export const scopeTypeVariant: Record<string, BadgeVariant> = {
  shared: "info",
  global: "primary",
};

// User role → badge variant
export const userRoleVariant: Record<string, BadgeVariant> = {
  admin: "danger",
  user: "default",
};

// VCS provider → badge variant
export const vcsProviderVariant: Record<string, BadgeVariant> = {
  github: "default",
  gitlab: "warning",
  bitbucket: "info",
  gitea: "success",
  svn: "primary",
};
