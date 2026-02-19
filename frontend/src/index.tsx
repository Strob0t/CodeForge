/* @refresh reload */
import "./index.css";

import { Route, Router } from "@solidjs/router";
import { render } from "solid-js/web";

import App from "./App.tsx";
import LoginPage from "./features/auth/LoginPage.tsx";
import CostDashboardPage from "./features/costs/CostDashboardPage.tsx";
import DashboardPage from "./features/dashboard/DashboardPage.tsx";
import ModelsPage from "./features/llm/ModelsPage.tsx";
import ProjectDetailPage from "./features/project/ProjectDetailPage.tsx";

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
    </Router>
  ),
  root,
);
