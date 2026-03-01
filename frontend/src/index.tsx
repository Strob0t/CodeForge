/* @refresh reload */
import "./index.css";

import { Route, Router } from "@solidjs/router";
import { render } from "solid-js/web";

import App from "./App.tsx";
import ActivityPage from "./features/activity/ActivityPage.tsx";
import ChangePasswordPage from "./features/auth/ChangePasswordPage.tsx";
import ForgotPasswordPage from "./features/auth/ForgotPasswordPage.tsx";
import LoginPage from "./features/auth/LoginPage.tsx";
import ResetPasswordPage from "./features/auth/ResetPasswordPage.tsx";
import SetupPage from "./features/auth/SetupPage.tsx";
import BenchmarkPage from "./features/benchmarks/BenchmarkPage.tsx";
import CostDashboardPage from "./features/costs/CostDashboardPage.tsx";
import DashboardPage from "./features/dashboard/DashboardPage.tsx";
import KnowledgeBasesPage from "./features/knowledgebases/KnowledgeBasesPage.tsx";
import ModelsPage from "./features/llm/ModelsPage.tsx";
import MCPServersPage from "./features/mcp/MCPServersPage.tsx";
import ModesPage from "./features/modes/ModesPage.tsx";
import NotFoundPage from "./features/NotFoundPage.tsx";
import ProjectDetailPage from "./features/project/ProjectDetailPage.tsx";
import PromptEditorPage from "./features/prompts/PromptEditorPage.tsx";
import ScopesPage from "./features/scopes/ScopesPage.tsx";
import SettingsPage from "./features/settings/SettingsPage.tsx";
import TeamsPage from "./features/teams/TeamsPage.tsx";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Root element not found");
}

render(
  () => (
    <Router root={App}>
      <Route path="/login" component={LoginPage} />
      <Route path="/change-password" component={ChangePasswordPage} />
      <Route path="/setup" component={SetupPage} />
      <Route path="/forgot-password" component={ForgotPasswordPage} />
      <Route path="/reset-password" component={ResetPasswordPage} />
      <Route path="/" component={DashboardPage} />
      <Route path="/projects" component={DashboardPage} />
      <Route path="/projects/:id" component={ProjectDetailPage} />
      <Route path="/costs" component={CostDashboardPage} />
      <Route path="/models" component={ModelsPage} />
      <Route path="/modes" component={ModesPage} />
      <Route path="/activity" component={ActivityPage} />
      <Route path="/knowledge-bases" component={KnowledgeBasesPage} />
      <Route path="/scopes" component={ScopesPage} />
      <Route path="/teams" component={TeamsPage} />
      <Route path="/mcp" component={MCPServersPage} />
      <Route path="/prompts" component={PromptEditorPage} />
      <Route path="/settings" component={SettingsPage} />
      <Route path="/benchmarks" component={BenchmarkPage} />
      <Route path="*404" component={NotFoundPage} />
    </Router>
  ),
  root,
);
