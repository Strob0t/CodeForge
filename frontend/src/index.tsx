/* @refresh reload */
import { render } from "solid-js/web";
import { Route, Router } from "@solidjs/router";
import App from "./App.tsx";
import DashboardPage from "./features/dashboard/DashboardPage.tsx";
import ProjectDetailPage from "./features/project/ProjectDetailPage.tsx";
import ModelsPage from "./features/llm/ModelsPage.tsx";
import "./index.css";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Root element not found");
}

render(
  () => (
    <Router root={App}>
      <Route path="/" component={DashboardPage} />
      <Route path="/projects" component={DashboardPage} />
      <Route path="/projects/:id" component={ProjectDetailPage} />
      <Route path="/models" component={ModelsPage} />
    </Router>
  ),
  root,
);
