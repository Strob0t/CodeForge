/* @refresh reload */
import "./index.css";

import { Route, Router } from "@solidjs/router";
import { render } from "solid-js/web";

import App from "./App.tsx";
import ActivityPage from "./features/activity/ActivityPage.tsx";
import LoginPage from "./features/auth/LoginPage.tsx";
import CostDashboardPage from "./features/costs/CostDashboardPage.tsx";
import DashboardPage from "./features/dashboard/DashboardPage.tsx";
import ModelsPage from "./features/llm/ModelsPage.tsx";
import ModesPage from "./features/modes/ModesPage.tsx";
import ProjectDetailPage from "./features/project/ProjectDetailPage.tsx";
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
      <Route path="/" component={DashboardPage} />
      <Route path="/projects" component={DashboardPage} />
      <Route path="/projects/:id" component={ProjectDetailPage} />
      <Route path="/costs" component={CostDashboardPage} />
      <Route path="/models" component={ModelsPage} />
      <Route path="/modes" component={ModesPage} />
      <Route path="/activity" component={ActivityPage} />
      <Route path="/teams" component={TeamsPage} />
      <Route path="/settings" component={SettingsPage} />
    </Router>
  ),
  root,
);
